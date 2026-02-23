package ai

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"kubikles/pkg/events"

	"github.com/google/uuid"
)

const defaultRequestTimeout = 10 * time.Minute

// AIResponseEvent is emitted to the frontend via Wails events.
type AIResponseEvent struct {
	SessionID  string      `json:"sessionId"`
	Generation uint64      `json:"generation"`
	Chunk      string      `json:"chunk,omitempty"`
	Done       bool        `json:"done,omitempty"`
	Error      string      `json:"error,omitempty"`
	Usage      *TokenUsage `json:"usage,omitempty"`
}

// session tracks a single AI chat session.
type session struct {
	id              string
	clientID        string // WebSocket client that owns this session (for cleanup on disconnect)
	cancel          context.CancelFunc
	generation      uint64    // incremented per SendMessage to detect stale events
	hasHistory      bool      // true after first message sent
	messages        []Message // accumulated conversation history for stateless providers
	cliSession      Session   // live CLI process for persistent session mode
	systemPrompt    string    // cached for session restart
	model           string    // cached for session restart
	k8sContext      string    // cached for session restart
	allowedTools    []string  // cached for session restart
	allowedCommands []string  // cached for session restart
}

// Manager manages AI chat sessions and provider lifecycle.
type Manager struct {
	ctx        context.Context
	emitter    events.Emitter
	provider   Provider
	providerID string    // registry key of the active provider
	registry   *Registry // for on-demand provider creation
	sessions   map[string]*session
	mu         sync.RWMutex
}

// NewManager creates a new AI manager with the given provider.
func NewManager(provider Provider) *Manager {
	return &Manager{
		provider: provider,
		sessions: make(map[string]*session),
	}
}

// NewManagerWithRegistry creates a new AI manager using the first available
// provider from the given registry. Returns an error if no provider is available.
func NewManagerWithRegistry(registry *Registry) (*Manager, error) {
	provider, providerID, err := registry.GetFirstAvailableWithID()
	if err != nil {
		return nil, err
	}
	return &Manager{
		provider:   provider,
		providerID: providerID,
		registry:   registry,
		sessions:   make(map[string]*session),
	}, nil
}

// ProviderID returns the registry key of the active provider.
func (m *Manager) ProviderID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providerID
}

// Registry returns the manager's provider registry (or DefaultRegistry as fallback).
func (m *Manager) Registry() *Registry {
	if m.registry != nil {
		return m.registry
	}
	return DefaultRegistry
}

// parseModel splits a compound model string like "claude-cli/sonnet" into
// provider ID and model name. A bare string like "sonnet" returns ("", "sonnet")
// for backward compatibility.
func parseModel(compound string) (providerID, modelName string) {
	if idx := strings.IndexByte(compound, '/'); idx >= 0 {
		return compound[:idx], compound[idx+1:]
	}
	return "", compound
}

// SetContext sets the Wails app context for event emission.
// This creates a WailsEmitter for desktop mode.
func (m *Manager) SetContext(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ctx = ctx
	// Default to WailsEmitter if no custom emitter is set
	if m.emitter == nil {
		m.emitter = events.NewWailsEmitter(ctx)
	}
}

// SetEmitter sets a custom event emitter and context (used for server mode).
func (m *Manager) SetEmitter(emitter events.Emitter, ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitter = emitter
	m.ctx = ctx
}

// CheckProvider checks if the configured AI provider is available.
func (m *Manager) CheckProvider() (bool, string) {
	return m.provider.IsAvailable()
}

// ProviderName returns the name of the configured AI provider.
func (m *Manager) ProviderName() string {
	return m.provider.Name()
}

// StartSession creates a new session and returns its ID.
// clientID is the WebSocket client ID (for server mode cleanup on disconnect).
// Pass empty string for desktop mode where sessions are managed by the app lifecycle.
func (m *Manager) StartSession(clientID string) string {
	id := uuid.New().String()
	m.mu.Lock()
	m.sessions[id] = &session{id: id, clientID: clientID}
	m.mu.Unlock()
	return id
}

// SendMessage sends a message in a session asynchronously.
// Streams response via ai:response Wails events.
// model can be a compound string like "claude-cli/sonnet" or a bare model name.
// timeoutSeconds overrides the default timeout when > 0.
// Returns true if the request was successfully initiated, false if it failed validation.
func (m *Manager) SendMessage(sessionID, message, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, timeoutSeconds int) bool {
	// Parse compound model (e.g. "codex-cli/o3" → providerID="codex-cli", modelName="o3")
	wantProvider, modelName := parseModel(model)

	m.mu.Lock()
	sess, exists := m.sessions[sessionID]
	appCtx := m.ctx

	// Switch provider if the compound model requests a different one
	if wantProvider != "" && wantProvider != m.providerID && m.registry != nil {
		newProvider, err := m.registry.CreateProvider(wantProvider)
		if err != nil {
			m.mu.Unlock()
			m.emit(AIResponseEvent{SessionID: sessionID, Error: fmt.Sprintf("provider %q not available: %v", wantProvider, err), Done: true})
			return false
		}
		if avail, _ := newProvider.IsAvailable(); !avail {
			m.mu.Unlock()
			m.emit(AIResponseEvent{SessionID: sessionID, Error: fmt.Sprintf("provider %q is not available", wantProvider), Done: true})
			return false
		}
		// Close all existing CLI sessions (they're bound to the old provider)
		for _, s := range m.sessions {
			if s.cliSession != nil {
				s.cliSession.Close()
				s.cliSession = nil
			}
		}
		m.provider = newProvider
		m.providerID = wantProvider
	}
	m.mu.Unlock()

	if !exists {
		m.emit(AIResponseEvent{SessionID: sessionID, Error: "session not found", Done: true})
		return false
	}

	if appCtx == nil {
		m.emit(AIResponseEvent{SessionID: sessionID, Error: "app context not initialized", Done: true})
		return false
	}

	// Check if provider supports persistent sessions
	if m.provider.SupportsSession() {
		return m.sendMessagePersistent(sess, sessionID, message, systemPrompt, modelName, k8sContext, allowedTools, allowedCommands, timeoutSeconds)
	}

	// Fallback to one-shot mode for providers that don't support sessions
	m.sendMessageOneShot(sess, sessionID, message, systemPrompt, modelName, k8sContext, allowedTools, allowedCommands, timeoutSeconds, appCtx)
	return true
}

// sortedEqual checks if two string slices contain the same elements (order-independent).
func sortedEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := make([]string, len(a))
	bc := make([]string, len(b))
	copy(ac, a)
	copy(bc, b)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}

// sendMessagePersistent uses a persistent CLI session for fast message sending.
// Returns true if the message was successfully queued for sending.
// Note: timeoutSeconds is unused for persistent sessions (they handle timeouts internally)
func (m *Manager) sendMessagePersistent(sess *session, sessionID, message, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, _ int) bool {
	m.mu.Lock()
	sess.generation++

	// Restart session if model, allowed tools, or commands changed
	if sess.cliSession != nil {
		if sess.model != model || !sortedEqual(sess.allowedTools, allowedTools) || !sortedEqual(sess.allowedCommands, allowedCommands) {
			sess.cliSession.Close()
			sess.cliSession = nil
		}
	}

	// Clear dead sessions so we can start a fresh one
	if sess.cliSession != nil && !sess.cliSession.IsAlive() {
		sess.cliSession = nil
	}

	// Start persistent session on first message (or after restart)
	if sess.cliSession == nil {
		// Cache session params for potential restart
		sess.systemPrompt = systemPrompt
		sess.model = model
		sess.k8sContext = k8sContext
		sess.allowedTools = allowedTools
		sess.allowedCommands = allowedCommands

		// Use a fresh CLI session ID to avoid stale session file conflicts.
		// The manager's sessionID is for frontend routing; the CLI gets its own ID.
		cliSessionID := uuid.New().String()

		// Capture a reference for the onEvent closure to filter stale events.
		// The reference is set after StartSession returns (safe because m.mu is held).
		var thisSession Session

		// Create event handler that looks up current generation at emit time
		// This ensures events are always emitted with the correct generation
		onEvent := func(event StreamEvent) {
			m.mu.RLock()
			currentSess, exists := m.sessions[sessionID]
			// Ignore events from old (replaced) CLI sessions
			if !exists || currentSess.cliSession != thisSession {
				m.mu.RUnlock()
				return
			}
			currentGen := currentSess.generation
			m.mu.RUnlock()

			switch event.Type {
			case "text":
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: currentGen, Chunk: event.Content, Usage: event.Usage})
			case "done":
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: currentGen, Done: true, Usage: event.Usage})
			case "error":
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: currentGen, Error: event.Content})
			}
		}

		cliSession, err := m.provider.StartSession(cliSessionID, systemPrompt, model, k8sContext, allowedTools, allowedCommands, onEvent)
		if err != nil {
			gen := sess.generation
			m.mu.Unlock()
			m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Error: fmt.Sprintf("failed to start session: %v", err), Done: true})
			return false
		}
		thisSession = cliSession
		sess.cliSession = cliSession
	}
	cliSession := sess.cliSession
	gen := sess.generation
	m.mu.Unlock()

	// Send message asynchronously
	go func() {
		if err := cliSession.SendMessage(message); err != nil {
			m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Error: fmt.Sprintf("failed to send message: %v", err), Done: true})

			// Session failed, clear it so next message restarts
			m.mu.Lock()
			if s, ok := m.sessions[sessionID]; ok && s.cliSession == cliSession {
				s.cliSession.Close()
				s.cliSession = nil
			}
			m.mu.Unlock()
		}
	}()
	return true
}

// sendMessageOneShot uses the traditional one-process-per-message approach.
func (m *Manager) sendMessageOneShot(sess *session, sessionID, message, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, timeoutSeconds int, appCtx context.Context) {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	ctx, cancel := context.WithTimeout(appCtx, timeout)
	m.mu.Lock()
	// Cancel any in-progress request for this session (inside lock to avoid races)
	if sess.cancel != nil {
		sess.cancel()
	}
	sess.cancel = cancel
	sess.generation++
	gen := sess.generation
	isResume := sess.hasHistory
	history := make([]Message, len(sess.messages))
	copy(history, sess.messages)
	m.mu.Unlock()

	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[AI] panic in sendMessageOneShot: %v", r)
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Error: fmt.Sprintf("internal error: %v", r), Done: true})
			}
		}()

		req := Request{
			SessionID:       sessionID,
			Message:         message,
			SystemPrompt:    systemPrompt,
			Model:           model,
			IsResume:        isResume,
			History:         history,
			K8sContext:      k8sContext,
			AllowedTools:    allowedTools,
			AllowedCommands: allowedCommands,
		}

		// Collect assistant response text for history
		var responseText strings.Builder

		err := m.provider.SendMessage(ctx, req, func(event StreamEvent) {
			switch event.Type {
			case "text":
				responseText.WriteString(event.Content)
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Chunk: event.Content, Usage: event.Usage})
			case "done":
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Done: true, Usage: event.Usage})
			case "error":
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Error: event.Content})
			}
		})

		if err != nil {
			if ctx.Err() == context.Canceled {
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Done: true})
				return
			}
			if ctx.Err() == context.DeadlineExceeded {
				m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Error: "request timed out", Done: true})
				return
			}
			m.emit(AIResponseEvent{SessionID: sessionID, Generation: gen, Error: fmt.Sprintf("provider error: %v", err), Done: true})
			return
		}

		// Accumulate conversation history and mark session as having history
		m.mu.Lock()
		if s, ok := m.sessions[sessionID]; ok {
			s.hasHistory = true
			s.messages = append(s.messages,
				Message{Role: "user", Content: message},
			)
			if resp := responseText.String(); resp != "" {
				s.messages = append(s.messages,
					Message{Role: "assistant", Content: resp},
				)
			}
		}
		m.mu.Unlock()
	}()
}

// CancelRequest cancels the in-progress request for a session.
func (m *Manager) CancelRequest(sessionID string) {
	m.mu.RLock()
	sess, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if exists && sess.cancel != nil {
		sess.cancel()
	}
}

// ClearSession resets a session by creating a new ID internally.
// Returns the new session ID.
func (m *Manager) ClearSession(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Preserve client ownership when creating new session
	var clientID string
	if sess, exists := m.sessions[sessionID]; exists {
		clientID = sess.clientID
		if sess.cancel != nil {
			sess.cancel()
		}
		if sess.cliSession != nil {
			sess.cliSession.Close()
		}
		delete(m.sessions, sessionID)
	}

	// Create a fresh session with same client ownership
	newID := uuid.New().String()
	m.sessions[newID] = &session{id: newID, clientID: clientID}
	return newID
}

// CloseSession cleans up a session.
func (m *Manager) CloseSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, exists := m.sessions[sessionID]; exists {
		if sess.cancel != nil {
			sess.cancel()
		}
		if sess.cliSession != nil {
			sess.cliSession.Close()
		}
		delete(m.sessions, sessionID)
	}
}

// CloseAllSessions closes all sessions (called on app shutdown).
func (m *Manager) CloseAllSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sess := range m.sessions {
		if sess.cancel != nil {
			sess.cancel()
		}
		if sess.cliSession != nil {
			sess.cliSession.Close()
		}
	}
	m.sessions = make(map[string]*session)
}

// OnClientDisconnect implements server.DisconnectListener.
// Cleans up all AI sessions owned by the disconnected client.
func (m *Manager) OnClientDisconnect(clientID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var toDelete []string
	for id, sess := range m.sessions {
		if sess.clientID == clientID {
			if sess.cancel != nil {
				sess.cancel()
			}
			if sess.cliSession != nil {
				sess.cliSession.Close()
			}
			toDelete = append(toDelete, id)
		}
	}

	for _, id := range toDelete {
		delete(m.sessions, id)
	}

	if len(toDelete) > 0 {
		// Log cleanup for debugging
		fmt.Printf("AI Manager: cleaned up %d session(s) for disconnected client %s\n", len(toDelete), clientID)
	}
}

// emit sends an AIResponseEvent to the frontend via the configured emitter.
func (m *Manager) emit(event AIResponseEvent) {
	m.mu.RLock()
	emitter := m.emitter
	m.mu.RUnlock()

	if emitter != nil {
		emitter.Emit("ai:response", event)
	}
}
