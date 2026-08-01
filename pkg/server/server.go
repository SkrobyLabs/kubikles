package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
)

// wsClient wraps a WebSocket connection with a write mutex for thread-safe writes
type wsClient struct {
	id      string // unique client ID for session tracking
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (c *wsClient) WriteJSON(v interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(v)
}

func (c *wsClient) Close() error {
	return c.conn.Close()
}

// DisconnectListener is notified when a WebSocket client disconnects.
// Components that manage per-client resources (AI sessions, terminals, watchers)
// can implement this to clean up when clients disconnect.
type DisconnectListener interface {
	OnClientDisconnect(clientID string)
}

// Server handles HTTP and WebSocket connections for server mode
type Server struct {
	caller                MethodCaller
	assets                embed.FS
	options               Options
	handler               http.Handler
	httpServer            *http.Server
	constructionErr       error
	listenFunc            func() (net.Listener, error)
	clients               map[*wsClient]bool
	clientsMu             sync.RWMutex
	broadcast             chan Event
	done                  chan struct{} // closed when server is shutting down
	upgrader              websocket.Upgrader
	disconnectListeners   []DisconnectListener
	listenersMu           sync.RWMutex
	clientCounter         uint64 // for generating unique client IDs
	broadcastOnce         sync.Once
	quiesceOnce           sync.Once
	closeOnce             sync.Once
	closeErr              error
	quiescing             atomic.Bool
	ownedShutdown         atomic.Bool
	afterWebSocketUpgrade func()
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
	options := CompatibilityOptions(port, nil)
	server, err := NewWithOptions(caller, assets, options)
	if err == nil {
		return server
	}
	server = newServer(caller, assets, options)
	server.constructionErr = err
	return server
}

// NewWithOptions validates and constructs a server with a prebuilt handler.
func NewWithOptions(caller MethodCaller, assets embed.FS, options Options) (*Server, error) {
	if err := validateOptions(options); err != nil {
		return nil, err
	}
	server := newServer(caller, assets, options)
	handler, err := server.buildHandler()
	if err != nil {
		return nil, err
	}
	server.handler = handler
	server.httpServer.Handler = handler
	return server, nil
}

func newServer(caller MethodCaller, assets embed.FS, options Options) *Server {
	server := &Server{
		caller:    caller,
		assets:    assets,
		options:   options,
		clients:   make(map[*wsClient]bool),
		broadcast: make(chan Event, 100),
		done:      make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins in server mode
			},
		},
	}
	server.httpServer = &http.Server{
		Addr:         options.ListenAddress,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return server
}

// Handler returns the server's prebuilt HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Listen creates the configured TCP listener.
func (s *Server) Listen() (net.Listener, error) {
	if s.constructionErr != nil {
		return nil, s.constructionErr
	}
	if s.listenFunc != nil {
		listener, err := s.listenFunc()
		if err != nil {
			return nil, fmt.Errorf("listen on %s: %w", s.options.ListenAddress, err)
		}
		return listener, nil
	}
	listener, err := net.Listen("tcp", s.options.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", s.options.ListenAddress, err)
	}
	return listener, nil
}

// Serve serves an already-created listener and reports unexpected failures.
func (s *Server) Serve(listener net.Listener) error {
	if s.constructionErr != nil {
		return s.constructionErr
	}
	s.broadcastOnce.Do(func() { go s.handleBroadcast() })
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) && s.ownedShutdown.Load() {
		return nil
	}
	if err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

// Quiesce immediately makes readiness and protected Accelerator routes unavailable.
func (s *Server) Quiesce() {
	s.quiesceOnce.Do(func() {
		s.quiescing.Store(true)
		if s.options.AcceleratorSessions != nil {
			s.options.AcceleratorSessions.Quiesce()
		}
	})
}

// Close gracefully shuts down the server. Concurrent calls share the first result.
func (s *Server) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.Quiesce()
		if s.options.AcceleratorSessions != nil {
			if err := s.options.AcceleratorSessions.Close(ctx); err != nil {
				s.closeErr = err
			}
		}
		s.ownedShutdown.Store(true)
		close(s.done)
		s.clientsMu.Lock()
		clients := make([]*wsClient, 0, len(s.clients))
		for client := range s.clients {
			clients = append(clients, client)
			delete(s.clients, client)
		}
		s.clientsMu.Unlock()
		for _, client := range clients {
			_ = client.Close()
		}
		if s.httpServer != nil {
			if err := s.httpServer.Shutdown(ctx); err != nil {
				s.closeErr = fmt.Errorf("shutdown HTTP server: %w", err)
			}
		}
	})
	return s.closeErr
}

// Run listens, serves, and gracefully shuts down on context cancellation.
func (s *Server) Run(ctx context.Context) error {
	listener, err := s.Listen()
	if err != nil {
		return err
	}
	log.Printf("Server mode: listening on http://%s", listener.Addr())
	serveResult := make(chan error, 1)
	go func() { serveResult <- s.Serve(listener) }()

	select {
	case serveErr := <-serveResult:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		closeErr := s.Close(shutdownCtx)
		if serveErr != nil {
			return serveErr
		}
		return closeErr
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		closeErr := s.Close(shutdownCtx)
		cancel()
		serveErr := <-serveResult
		if closeErr != nil {
			return closeErr
		}
		return serveErr
	}
}

// EmitEvent sends an event to all connected WebSocket clients
func (s *Server) EmitEvent(name string, data interface{}) {
	if s.options.BoundaryMode == BoundaryModeAccelerator && s.options.AcceleratorSessions != nil {
		s.options.AcceleratorSessions.EmitEvent(name, data)
		return
	}
	select {
	case s.broadcast <- Event{Type: "event", Name: name, Data: data}:
	case <-s.done:
		// Server is shutting down, drop the event
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

func (s *Server) handleBroadcast() {
	for {
		var event Event
		select {
		case <-s.done:
			return
		case event = <-s.broadcast:
		}
		// Snapshot clients before writing so a stalled WebSocket cannot block shutdown.
		s.clientsMu.RLock()
		clients := make([]*wsClient, 0, len(s.clients))
		for client := range s.clients {
			clients = append(clients, client)
		}
		s.clientsMu.RUnlock()

		var failed []*wsClient
		for _, client := range clients {
			if err := client.WriteJSON(event); err != nil {
				log.Printf("WebSocket write error: %v", err)
				failed = append(failed, client)
			}
		}

		// Remove failed clients under the write lock, but close their sockets afterwards.
		if len(failed) > 0 {
			s.clientsMu.Lock()
			toClose := make([]*wsClient, 0, len(failed))
			for _, client := range failed {
				if s.clients[client] {
					delete(s.clients, client)
					toClose = append(toClose, client)
				}
			}
			s.clientsMu.Unlock()
			for _, client := range toClose {
				_ = client.Close()
			}
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	if s.afterWebSocketUpgrade != nil {
		s.afterWebSocketUpgrade()
	}

	// Generate unique client ID
	clientNum := atomic.AddUint64(&s.clientCounter, 1)
	clientID := fmt.Sprintf("ws-%d", clientNum)

	client := &wsClient{id: clientID, conn: conn}

	s.clientsMu.Lock()
	select {
	case <-s.done:
		s.clientsMu.Unlock()
		_ = client.Close()
		return
	default:
	}
	s.clients[client] = true
	clientCount := len(s.clients)
	s.clientsMu.Unlock()

	log.Printf("WebSocket client %s connected (total: %d)", clientID, clientCount)

	// Send initial connection event with client ID
	_ = client.WriteJSON(Event{Type: "event", Name: "connected", Data: map[string]interface{}{
		"serverMode": true,
		"clientId":   clientID,
	}})

	// Handle incoming messages (for future bidirectional communication)
	go func() {
		defer func() {
			s.clientsMu.Lock()
			delete(s.clients, client)
			clientCount := len(s.clients)
			s.clientsMu.Unlock()
			client.Close()
			log.Printf("WebSocket client %s disconnected (total: %d)", clientID, clientCount)

			// Notify listeners so they can clean up per-client resources
			s.notifyDisconnect(clientID)
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
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
			s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
			return
		}
		methodName = req.Method
		args = req.Args
	} else {
		// Handle /api/MethodName with args array in body
		methodName = strings.Split(path, "/")[0]
		if r.Method == "POST" && r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&args); err != nil && err.Error() != "EOF" {
				s.writeError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
				return
			}
		}
	}

	if methodName == "" {
		s.writeError(w, http.StatusBadRequest, "Method name required")
		return
	}

	// Call method via the MethodCaller interface
	result, err := s.caller.CallMethod(agent.LocalCallContext(), methodName, args)
	if err != nil {
		// Check if it's a "not found" error for proper HTTP status
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Return result in format expected by frontend adapter
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": result,
	})
}

func (s *Server) writeError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	_ = writeJSON(w, map[string]interface{}{
		"error": message,
	})
}
