package terminal

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Manager manages terminal sessions using Wails events for IPC
type Manager struct {
	ctx      context.Context
	sessions map[string]*Session
	mu       sync.RWMutex
}

// SessionOptions contains options for starting a terminal session
type SessionOptions struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Context   string `json:"context"`
	Command   string `json:"command"` // Optional custom command (e.g., "nsenter")
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
		sessions: make(map[string]*Session),
	}
}

// SetContext sets the Wails app context for event emission
func (m *Manager) SetContext(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
}

// StartSession starts a new terminal session and returns the session ID
func (m *Manager) StartSession(opts SessionOptions) (string, error) {
	if opts.Namespace == "" || opts.Pod == "" {
		return "", fmt.Errorf("namespace and pod are required")
	}

	sessionID := uuid.New().String()

	m.mu.Lock()
	ctx := m.ctx
	m.mu.Unlock()

	if ctx == nil {
		return "", fmt.Errorf("manager context not initialized")
	}

	session, err := newSession(sessionID, opts, func(event TerminalEvent) {
		m.mu.RLock()
		c := m.ctx
		m.mu.RUnlock()
		if c != nil {
			runtime.EventsEmit(c, "terminal:output", event)
		}
	})
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
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
	m.mu.Unlock()

	for _, s := range sessions {
		s.Close()
	}
}
