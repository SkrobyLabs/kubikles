package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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
	caller              MethodCaller
	assets              embed.FS
	port                int
	clients             map[*wsClient]bool
	clientsMu           sync.RWMutex
	broadcast           chan Event
	done                chan struct{} // closed when server is shutting down
	upgrader            websocket.Upgrader
	disconnectListeners []DisconnectListener
	listenersMu         sync.RWMutex
	clientCounter       uint64 // for generating unique client IDs
}

// Event represents a WebSocket event to send to clients
type Event struct {
	Type string      `json:"type"`
	Name string      `json:"name"`
	Data interface{} `json:"data"`
}

// New creates a new server instance.
// The app parameter should be a struct whose public methods will be exposed via the API.
func New(app interface{}, assets embed.FS, port int) *Server {
	return &Server{
		caller:    NewReflectMethodCaller(app),
		assets:    assets,
		port:      port,
		clients:   make(map[*wsClient]bool),
		broadcast: make(chan Event, 100),
		done:      make(chan struct{}),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins in server mode
			},
		},
	}
}

// Run starts the HTTP server
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()

	// Serve WebSocket endpoint
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Serve API endpoints
	mux.HandleFunc("/api/", s.handleAPI)

	// Serve static files from embedded assets
	subFS, err := fs.Sub(s.assets, "frontend/dist")
	if err != nil {
		return fmt.Errorf("failed to create sub filesystem: %w", err)
	}
	fileServer := http.FileServer(http.FS(subFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// For SPA routing, serve index.html for non-file requests
		path := r.URL.Path
		if path != "/" && !strings.Contains(path, ".") {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start broadcast handler
	go s.handleBroadcast()

	// Start server in goroutine
	go func() {
		log.Printf("Server mode: listening on http://localhost:%d", s.port)
		log.Printf("Open in your browser: http://localhost:%d", s.port)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Signal shutdown to prevent new events being queued
	close(s.done)

	// Close broadcast channel to stop the handleBroadcast goroutine
	close(s.broadcast)

	return server.Shutdown(shutdownCtx)
}

// EmitEvent sends an event to all connected WebSocket clients
func (s *Server) EmitEvent(name string, data interface{}) {
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
	for event := range s.broadcast {
		// Collect failed clients while holding read lock
		var failed []*wsClient
		s.clientsMu.RLock()
		for client := range s.clients {
			if err := client.WriteJSON(event); err != nil {
				log.Printf("WebSocket write error: %v", err)
				failed = append(failed, client)
			}
		}
		s.clientsMu.RUnlock()

		// Remove failed clients with write lock
		if len(failed) > 0 {
			s.clientsMu.Lock()
			for _, client := range failed {
				client.Close()
				delete(s.clients, client)
			}
			s.clientsMu.Unlock()
		}
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Generate unique client ID
	clientNum := atomic.AddUint64(&s.clientCounter, 1)
	clientID := fmt.Sprintf("ws-%d", clientNum)

	client := &wsClient{id: clientID, conn: conn}

	s.clientsMu.Lock()
	s.clients[client] = true
	s.clientsMu.Unlock()

	log.Printf("WebSocket client %s connected (total: %d)", clientID, len(s.clients))

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
	result, err := s.caller.CallMethod(methodName, args)
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
	})
}
