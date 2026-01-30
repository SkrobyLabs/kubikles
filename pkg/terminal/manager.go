package terminal

import (
	"context"
	"fmt"
	"sync"

	"kubikles/pkg/events"

	"github.com/google/uuid"
)

// Manager manages terminal sessions using events for IPC
type Manager struct {
	ctx           context.Context
	sessions      map[string]*Session
	sessionClient map[string]string // sessionID -> clientID for cleanup on disconnect
	mu            sync.RWMutex
	emitter       events.Emitter
}

// SessionOptions contains options for starting a terminal session
type SessionOptions struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Context   string `json:"context"`
	Command   string `json:"command"`  // Optional custom command (e.g., "nsenter")
	ClientID  string `json:"clientId"` // WebSocket client ID for server mode cleanup
}

// TerminalEvent is emitted to the frontend for terminal output
type TerminalEvent struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data,omitempty"`  // Terminal output as string
	Done      bool   `json:"done,omitempty"`  // Session ended
	Error     string `json:"error,omitempty"` // Error message
}

// NewManager creates a new terminal manager
func NewManager() *Manager {
	return &Manager{
		sessions:      make(map[string]*Session),
		sessionClient: make(map[string]string),
	}
}

// SetContext sets the app context (used for lifecycle management)
func (m *Manager) SetContext(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
}

// SetEmitter sets the event emitter for terminal output
func (m *Manager) SetEmitter(emitter events.Emitter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitter = emitter
}

// emitEvent emits an event using the configured emitter
func (m *Manager) emitEvent(name string, data interface{}) {
	m.mu.RLock()
	emitter := m.emitter
	m.mu.RUnlock()

	if emitter != nil {
		emitter.Emit(name, data)
	}
}

// StartSession starts a new terminal session and returns the session ID
func (m *Manager) StartSession(opts SessionOptions) (string, error) {
	if opts.Namespace == "" || opts.Pod == "" {
		return "", fmt.Errorf("namespace and pod are required")
	}

	sessionID := uuid.New().String()

	m.mu.RLock()
	emitter := m.emitter
	m.mu.RUnlock()

	if emitter == nil {
		return "", fmt.Errorf("manager emitter not initialized")
	}

	session, err := newSession(sessionID, opts, func(event TerminalEvent) {
		m.emitEvent("terminal:output", event)
	})
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
	if opts.ClientID != "" {
		m.sessionClient[sessionID] = opts.ClientID
	}
	m.mu.Unlock()

	return sessionID, nil
}

// SendInput sends input data to a terminal session
func (m *Manager) SendInput(sessionID string, data []byte) error {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return session.Write(data)
}

// Resize resizes the terminal for a session
func (m *Manager) Resize(sessionID string, cols, rows int) error {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	return session.Resize(cols, rows)
}

// CloseSession closes a specific terminal session
func (m *Manager) CloseSession(sessionID string) error {
	m.mu.Lock()
	session, exists := m.sessions[sessionID]
	if exists {
		delete(m.sessions, sessionID)
		delete(m.sessionClient, sessionID)
	}
	m.mu.Unlock()

	if !exists {
		return nil // Already closed
	}

	session.Close()
	return nil
}

// CloseAllSessions closes all terminal sessions (called on app shutdown)
func (m *Manager) CloseAllSessions() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*Session)
	m.sessionClient = make(map[string]string)
	m.mu.Unlock()

	for _, s := range sessions {
		s.Close()
	}
}

// OnClientDisconnect implements server.DisconnectListener.
// Cleans up all terminal sessions owned by the disconnected client.
func (m *Manager) OnClientDisconnect(clientID string) {
	m.mu.Lock()
	var toClose []*Session
	var toDelete []string

	for sessionID, client := range m.sessionClient {
		if client == clientID {
			if session, exists := m.sessions[sessionID]; exists {
				toClose = append(toClose, session)
			}
			toDelete = append(toDelete, sessionID)
		}
	}

	for _, id := range toDelete {
		delete(m.sessions, id)
		delete(m.sessionClient, id)
	}
	m.mu.Unlock()

	for _, session := range toClose {
		session.Close()
	}

	if len(toDelete) > 0 {
		fmt.Printf("Terminal Manager: cleaned up %d session(s) for disconnected client %s\n", len(toDelete), clientID)
	}
}
