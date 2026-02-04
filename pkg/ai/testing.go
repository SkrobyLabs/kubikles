package ai

import (
	"context"
	"sync"
)

// MockProvider is a test double for AI providers.
type MockProvider struct {
	name              string
	available         bool
	availableStatus   string
	supportsSession   bool
	capabilities      ProviderCapabilities
	sendMessageFunc   func(ctx context.Context, req Request, onChunk func(StreamEvent)) error
	startSessionFunc  func(sessionID, systemPrompt, model, k8sContext string, allowedTools []string, onEvent func(StreamEvent)) (Session, error)
	mu                sync.Mutex
	sendMessageCalls  []Request
	startSessionCalls []mockStartSessionCall
}

type mockStartSessionCall struct {
	SessionID    string
	SystemPrompt string
	Model        string
	K8sContext   string
	AllowedTools []string
}

// NewMockProvider creates a new mock provider with default settings.
func NewMockProvider() *MockProvider {
	return &MockProvider{
		name:            "Mock Provider",
		available:       true,
		availableStatus: "Mock provider available",
		supportsSession: true,
		capabilities: ProviderCapabilities{
			SupportsStreaming: true,
			SupportsSessions:  true,
			MaxContextLength:  100000,
		},
	}
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) SetName(name string) {
	m.name = name
}

func (m *MockProvider) IsAvailable() (bool, string) {
	return m.available, m.availableStatus
}

func (m *MockProvider) SetAvailable(available bool, status string) {
	m.available = available
	m.availableStatus = status
}

func (m *MockProvider) SupportsSession() bool {
	return m.supportsSession
}

func (m *MockProvider) SetSupportsSession(supports bool) {
	m.supportsSession = supports
}

func (m *MockProvider) SendMessage(ctx context.Context, req Request, onChunk func(StreamEvent)) error {
	m.mu.Lock()
	m.sendMessageCalls = append(m.sendMessageCalls, req)
	m.mu.Unlock()

	if m.sendMessageFunc != nil {
		return m.sendMessageFunc(ctx, req, onChunk)
	}

	// Default: emit a simple response
	onChunk(StreamEvent{Type: "text", Content: "Mock response"})
	onChunk(StreamEvent{Type: "done"})
	return nil
}

func (m *MockProvider) SetSendMessageFunc(fn func(ctx context.Context, req Request, onChunk func(StreamEvent)) error) {
	m.sendMessageFunc = fn
}

func (m *MockProvider) StartSession(sessionID, systemPrompt, model, k8sContext string, allowedTools []string, onEvent func(StreamEvent)) (Session, error) {
	m.mu.Lock()
	m.startSessionCalls = append(m.startSessionCalls, mockStartSessionCall{
		SessionID:    sessionID,
		SystemPrompt: systemPrompt,
		Model:        model,
		K8sContext:   k8sContext,
		AllowedTools: allowedTools,
	})
	m.mu.Unlock()

	if m.startSessionFunc != nil {
		return m.startSessionFunc(sessionID, systemPrompt, model, k8sContext, allowedTools, onEvent)
	}

	return NewMockSession(onEvent), nil
}

func (m *MockProvider) SetStartSessionFunc(fn func(sessionID, systemPrompt, model, k8sContext string, allowedTools []string, onEvent func(StreamEvent)) (Session, error)) {
	m.startSessionFunc = fn
}

func (m *MockProvider) Capabilities() ProviderCapabilities {
	return m.capabilities
}

func (m *MockProvider) SetCapabilities(caps ProviderCapabilities) {
	m.capabilities = caps
}

// GetSendMessageCalls returns all recorded SendMessage calls.
func (m *MockProvider) GetSendMessageCalls() []Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Request, len(m.sendMessageCalls))
	copy(result, m.sendMessageCalls)
	return result
}

// GetStartSessionCalls returns all recorded StartSession calls.
func (m *MockProvider) GetStartSessionCalls() []mockStartSessionCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockStartSessionCall, len(m.startSessionCalls))
	copy(result, m.startSessionCalls)
	return result
}

// MockSession is a test double for AI sessions.
type MockSession struct {
	emitFunc        func(StreamEvent)
	messages        []string
	closed          bool
	sendMessageFunc func(message string) error
	mu              sync.Mutex
}

// NewMockSession creates a new mock session.
func NewMockSession(emitFunc func(StreamEvent)) *MockSession {
	return &MockSession{
		emitFunc: emitFunc,
	}
}

func (s *MockSession) SendMessage(message string) error {
	s.mu.Lock()
	s.messages = append(s.messages, message)
	sendFunc := s.sendMessageFunc
	emitFunc := s.emitFunc
	s.mu.Unlock()

	if sendFunc != nil {
		return sendFunc(message)
	}

	// Default: emit a simple response asynchronously
	go func() {
		if emitFunc != nil {
			emitFunc(StreamEvent{Type: "text", Content: "Mock session response"})
			emitFunc(StreamEvent{Type: "done"})
		}
	}()

	return nil
}

func (s *MockSession) SetSendMessageFunc(fn func(message string) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sendMessageFunc = fn
}

func (s *MockSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

// IsClosed returns whether the session has been closed.
func (s *MockSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// GetMessages returns all messages sent to this session.
func (s *MockSession) GetMessages() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.messages))
	copy(result, s.messages)
	return result
}

// EmitEvent allows tests to manually emit events through this session.
func (s *MockSession) EmitEvent(event StreamEvent) {
	s.mu.Lock()
	emitFunc := s.emitFunc
	s.mu.Unlock()

	if emitFunc != nil {
		emitFunc(event)
	}
}
