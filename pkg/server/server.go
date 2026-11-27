package server

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kubikles/pkg/compressedassets"

	"github.com/gorilla/websocket"
)

// DisconnectListener is notified when a WebSocket client disconnects.
// Components that manage per-client resources (AI sessions, terminals, watchers)
// can implement this to clean up when clients disconnect.
type DisconnectListener interface {
	OnClientDisconnect(clientID string)
}

// IdleListener is an optional extension of DisconnectListener for resources
// that are shared by all clients rather than owned by one (log streams). It is
// called once no client has been connected for idleGrace, so a page reload or a
// brief network drop does not tear them down. OnNoClients runs while the server
// holds its client lock, so a client cannot connect between the check and the
// call; it must be quick and must not call back into the server.
type IdleListener interface {
	OnNoClients()
}

// idleGrace is how long the server waits with no clients before telling
// IdleListeners. A variable so tests can shorten it.
var idleGrace = 30 * time.Second

// Server handles HTTP and WebSocket connections for server mode
type Server struct {
	caller              MethodCaller
	assets              embed.FS
	port                int
	clients             map[*wsClient]bool
	clientsMu           sync.RWMutex
	fanoutMu            sync.Mutex
	closeOnce           sync.Once
	wg                  sync.WaitGroup // client reader and writer goroutines
	closing             bool           // guarded by clientsMu
	done                chan struct{}  // closed when server is shutting down
	upgrader            websocket.Upgrader
	disconnectListeners []DisconnectListener
	listenersMu         sync.RWMutex
	clientCounter       uint64       // for generating unique client IDs
	bulk                http.Handler // optional /_kubikles/ bulk transport
	idleTimer           *time.Timer  // guarded by clientsMu; pending IdleListener call
	idleGen             uint64       // guarded by clientsMu; bumped on every arm, connect and close
}

// Event represents a WebSocket event to send to clients
type Event struct {
	Type string      `json:"type"`
	Name string      `json:"name"`
	Data interface{} `json:"data"`
}

// New creates a new server instance.
// The caller parameter implements MethodCaller for dispatching API calls.
func New(caller MethodCaller, assets embed.FS, port int) *Server {
	return &Server{
		caller:  caller,
		assets:  assets,
		port:    port,
		clients: make(map[*wsClient]bool),
		done:    make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins in server mode
			},
		},
	}
}

// WithBulk mounts the bulk transport handler under /_kubikles/. The handler
// owns its own guards, gzip and per-chunk write deadlines.
func (s *Server) WithBulk(h http.Handler) *Server {
	s.bulk = h
	return s
}

// Run starts the HTTP server
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()

	// Serve WebSocket endpoint
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Serve API endpoints
	mux.HandleFunc("/api/", s.handleAPI)

	// Bulk read transport (GET only, token guarded, streams past WriteTimeout).
	if s.bulk != nil {
		mux.Handle("/_kubikles/", s.bulk)
	}

	// Serve static files from embedded assets (pre-compressed with gzip)
	subFS, err := fs.Sub(s.assets, "frontend/dist")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem: %w", err)
	}
	fileServer := compressedassets.GzipAwareFileServer(subFS)
	mux.Handle("/", fileServer)

	server := &http.Server{
		// KUBIKLES_BIND_ADDR restricts the listener to one interface (for
		// example 127.0.0.1); empty keeps the default of all interfaces.
		Addr:         net.JoinHostPort(os.Getenv("KUBIKLES_BIND_ADDR"), strconv.Itoa(s.port)),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server mode: listening on %s", server.Addr)
		log.Printf("Open in your browser: http://%s", browserAddr(server.Addr))
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.closeClients()

	err = server.Shutdown(shutdownCtx)
	s.wg.Wait()
	return err
}

// EmitEvent sends an event to all connected WebSocket clients. The event is
// encoded once and queued per client without blocking: a client that cannot
// keep up is disconnected, never allowed to stall the caller.
func (s *Server) EmitEvent(name string, data interface{}) {
	select {
	case <-s.done:
		return
	default:
	}
	s.clientsMu.RLock()
	n := len(s.clients)
	s.clientsMu.RUnlock()
	if n == 0 {
		return
	}

	payload, err := json.Marshal(Event{Type: "event", Name: name, Data: data})
	if err != nil {
		log.Printf("WebSocket event %q encode error: %v", name, err)
		return
	}
	pm, err := websocket.NewPreparedMessage(websocket.TextMessage, payload)
	if err != nil {
		return
	}
	s.fanout(queuedMessage{msg: pm, size: int64(len(payload))})
}

// fanout holds the read lock only for non-blocking enqueues. fanoutMu keeps
// the per-client order identical to the global emit order.
func (s *Server) fanout(m queuedMessage) {
	s.fanoutMu.Lock()
	defer s.fanoutMu.Unlock()
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	for c := range s.clients {
		if !c.enqueue(m) {
			log.Printf("WebSocket client %s dropped: send queue full or closed", c.id)
		}
	}
}

// AddDisconnectListener registers a listener to be notified when clients disconnect.
// This allows components (AI manager, terminal manager, etc.) to clean up per-client resources.
func (s *Server) AddDisconnectListener(listener DisconnectListener) {
	s.listenersMu.Lock()
	defer s.listenersMu.Unlock()
	s.disconnectListeners = append(s.disconnectListeners, listener)
}

// notifyDisconnect notifies all registered listeners that a client has disconnected.
func (s *Server) notifyDisconnect(clientID string) {
	s.listenersMu.RLock()
	listeners := make([]DisconnectListener, len(s.disconnectListeners))
	copy(listeners, s.disconnectListeners)
	s.listenersMu.RUnlock()

	for _, listener := range listeners {
		go listener.OnClientDisconnect(clientID)
	}
}

// notifyIdle runs when the idle grace period ends. gen is the generation the
// timer was armed with: a callback of a timer that was replaced or cancelled
// (it may already have fired when Stop ran) sees a newer generation and does
// nothing, so it cannot clear a newer timer or fire without its grace period.
// The check and the listener calls share one critical section, so a client that
// connects cannot slip in between and have its new streams cancelled.
func (s *Server) notifyIdle(gen uint64) {
	s.listenersMu.RLock()
	listeners := make([]DisconnectListener, len(s.disconnectListeners))
	copy(listeners, s.disconnectListeners)
	s.listenersMu.RUnlock()

	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
	if gen != s.idleGen || len(s.clients) != 0 || s.closing {
		return
	}
	s.idleTimer = nil
	for _, listener := range listeners {
		if il, ok := listener.(IdleListener); ok {
			il.OnNoClients()
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	select {
	case <-s.done:
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	default:
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	clientID := fmt.Sprintf("ws-%d", atomic.AddUint64(&s.clientCounter, 1))
	client := newWSClient(clientID, conn)

	// Queue the hello before registering so it is always the first message.
	if payload, err := json.Marshal(Event{Type: "event", Name: "connected", Data: map[string]interface{}{
		"serverMode": true,
		"clientId":   clientID,
	}}); err == nil {
		if pm, err := websocket.NewPreparedMessage(websocket.TextMessage, payload); err == nil {
			client.enqueue(queuedMessage{msg: pm, size: int64(len(payload))})
		}
	}

	s.clientsMu.Lock()
	if s.closing {
		s.clientsMu.Unlock()
		_ = conn.Close()
		return
	}
	s.clients[client] = true
	s.idleGen++
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	total := len(s.clients)
	s.wg.Add(2)
	s.clientsMu.Unlock()

	log.Printf("WebSocket client %s connected (total: %d)", clientID, total)

	go func() {
		defer s.wg.Done()
		client.writeLoop()
	}()
	go func() {
		defer s.wg.Done()
		client.readLoop()
		client.kill(nil)
		s.clientsMu.Lock()
		delete(s.clients, client)
		count := len(s.clients)
		if count == 0 && !s.closing {
			if s.idleTimer != nil {
				s.idleTimer.Stop()
			}
			s.idleGen++
			gen := s.idleGen
			s.idleTimer = time.AfterFunc(idleGrace, func() { s.notifyIdle(gen) })
		}
		s.clientsMu.Unlock()
		log.Printf("WebSocket client %s disconnected (total: %d)", clientID, count)
		// Notify listeners so they can clean up per-client resources
		s.notifyDisconnect(clientID)
	}()
}

// APIRequest represents a JSON-RPC style API call
type APIRequest struct {
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args"`
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Limit request body to 64MB to prevent memory exhaustion
	// (large CRDs, ConfigMaps, and YAML payloads can be sizeable)
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)

	// Extract path after /api/
	path := strings.TrimPrefix(r.URL.Path, "/api/")

	var methodName string
	var args []json.RawMessage

	// Handle /api/call with JSON body {method, args}
	if path == "call" && r.Method == "POST" {
		var req APIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, r, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
			return
		}
		methodName = req.Method
		args = req.Args
	} else {
		// Handle /api/MethodName with args array in body
		methodName = strings.Split(path, "/")[0]
		if r.Method == "POST" && r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&args); err != nil && err.Error() != "EOF" {
				s.writeError(w, r, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
				return
			}
		}
	}

	if methodName == "" {
		s.writeError(w, r, http.StatusBadRequest, "Method name required")
		return
	}

	// Call method via the MethodCaller interface
	result, err := s.caller.CallMethod(methodName, args)
	if err != nil {
		// Check if it's a "not found" error for proper HTTP status
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, r, http.StatusNotFound, err.Error())
		} else {
			s.writeError(w, r, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Return result in format expected by frontend adapter
	s.writeJSON(w, r, http.StatusOK, map[string]interface{}{"data": result})
}

// writeJSON encodes into a pooled buffer so the body can be sized, and
// gzipped when large (lists can reach tens of MB).
func (s *Server) writeJSON(w http.ResponseWriter, r *http.Request, status int, v interface{}) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= 8<<20 {
			bufPool.Put(buf)
		}
	}()
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		buf.Reset()
		_ = json.NewEncoder(buf).Encode(map[string]interface{}{"error": fmt.Sprintf("encode response: %v", err)})
		status = http.StatusInternalServerError
	}
	writeBody(w, r, status, buf.Bytes())
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, message string) {
	s.writeJSON(w, r, status, map[string]interface{}{"error": message})
}

// closeClients stops accepting events and sockets and closes every client
// with a going-away frame. Idempotent.
func (s *Server) closeClients() {
	s.closeOnce.Do(func() { close(s.done) })
	s.clientsMu.Lock()
	s.closing = true
	s.idleGen++
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	clients := make([]*wsClient, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.clientsMu.Unlock()
	for _, c := range clients {
		c.kill(websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"))
	}
}

// browserAddr turns a listen address into one a browser can open: an empty or
// unspecified host (all interfaces) becomes localhost.
func browserAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}
