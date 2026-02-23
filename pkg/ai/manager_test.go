package ai

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// testEvent stores an emitted event for testing.
type testEvent struct {
	Name string
	Data interface{}
}

// mockEmitter collects emitted events for testing.
type mockEmitter struct {
	events []testEvent
	mu     sync.Mutex
}

func (m *mockEmitter) Emit(name string, data ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var eventData interface{}
	if len(data) > 0 {
		eventData = data[0]
	}
	m.events = append(m.events, testEvent{Name: name, Data: eventData})
}

func (m *mockEmitter) getEvents() []testEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]testEvent, len(m.events))
	copy(result, m.events)
	return result
}

func TestManager_StartSession(t *testing.T) {
	provider := NewMockProvider()
	manager := NewManager(provider)

	// Start a session
	sessionID := manager.StartSession("")
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Start another session - should get different ID
	sessionID2 := manager.StartSession("")
	if sessionID2 == "" || sessionID2 == sessionID {
		t.Fatal("expected different session ID")
	}
}

func TestManager_StartSession_WithClientID(t *testing.T) {
	provider := NewMockProvider()
	manager := NewManager(provider)

	clientID := "client-123"
	sessionID := manager.StartSession(clientID)
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}
}

func TestManager_CheckProvider(t *testing.T) {
	provider := NewMockProvider()
	provider.SetAvailable(true, "Provider ready")
	manager := NewManager(provider)

	available, status := manager.CheckProvider()
	if !available {
		t.Error("expected provider to be available")
	}
	if status != "Provider ready" {
		t.Errorf("expected status 'Provider ready', got %q", status)
	}

	// Test unavailable
	provider.SetAvailable(false, "Not installed")
	available, status = manager.CheckProvider()
	if available {
		t.Error("expected provider to be unavailable")
	}
	if status != "Not installed" {
		t.Errorf("expected status 'Not installed', got %q", status)
	}
}

func TestManager_ProviderName(t *testing.T) {
	provider := NewMockProvider()
	provider.SetName("Test Provider")
	manager := NewManager(provider)

	name := manager.ProviderName()
	if name != "Test Provider" {
		t.Errorf("expected name 'Test Provider', got %q", name)
	}
}

func TestManager_SendMessage_SessionNotFound(t *testing.T) {
	provider := NewMockProvider()
	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	// Send message to non-existent session
	manager.SendMessage("non-existent", "hello", "", "", "", nil, nil, 0)

	// Wait for event
	time.Sleep(10 * time.Millisecond)

	events := emitter.getEvents()
	if len(events) == 0 {
		t.Fatal("expected error event")
	}

	event := events[0].Data.(AIResponseEvent)
	if event.Error != "session not found" {
		t.Errorf("expected 'session not found' error, got %q", event.Error)
	}
}

func TestManager_SendMessage_NoContext(t *testing.T) {
	provider := NewMockProvider()
	manager := NewManager(provider)

	emitter := &mockEmitter{}
	// Don't set context - emitter but no ctx
	manager.SetEmitter(emitter, nil)

	sessionID := manager.StartSession("")
	manager.SendMessage(sessionID, "hello", "", "", "", nil, nil, 0)

	// Wait for event
	time.Sleep(10 * time.Millisecond)

	events := emitter.getEvents()
	if len(events) == 0 {
		t.Fatal("expected error event")
	}

	event := events[0].Data.(AIResponseEvent)
	if event.Error != "app context not initialized" {
		t.Errorf("expected 'app context not initialized' error, got %q", event.Error)
	}
}

func TestManager_SendMessage_PersistentSession(t *testing.T) {
	provider := NewMockProvider()
	provider.SetSupportsSession(true)

	var capturedSession *MockSession
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		capturedSession = NewMockSession(onEvent)
		return capturedSession, nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")
	manager.SendMessage(sessionID, "hello", "system", "model", "ctx", []string{"tool1"}, nil, 0)

	// Wait for session to start and message to be sent
	time.Sleep(50 * time.Millisecond)

	// Check session was started
	calls := provider.GetStartSessionCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 start session call, got %d", len(calls))
	}

	call := calls[0]
	if call.SystemPrompt != "system" {
		t.Errorf("expected system prompt 'system', got %q", call.SystemPrompt)
	}
	if call.Model != "model" {
		t.Errorf("expected model 'model', got %q", call.Model)
	}
	if call.K8sContext != "ctx" {
		t.Errorf("expected k8s context 'ctx', got %q", call.K8sContext)
	}

	// Check message was sent to session
	if capturedSession != nil {
		messages := capturedSession.GetMessages()
		if len(messages) != 1 || messages[0] != "hello" {
			t.Errorf("expected message 'hello', got %v", messages)
		}
	}
}

func TestManager_SendMessage_OneShotMode(t *testing.T) {
	provider := NewMockProvider()
	provider.SetSupportsSession(false)

	var receivedReq Request
	provider.SetSendMessageFunc(func(ctx context.Context, req Request, onChunk func(StreamEvent)) error {
		receivedReq = req
		onChunk(StreamEvent{Type: "text", Content: "response"})
		onChunk(StreamEvent{Type: "done"})
		return nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")
	manager.SendMessage(sessionID, "hello", "system", "model", "ctx", []string{"tool1"}, nil, 0)

	// Wait for response
	time.Sleep(50 * time.Millisecond)

	// Check request was made
	if receivedReq.Message != "hello" {
		t.Errorf("expected message 'hello', got %q", receivedReq.Message)
	}
	if receivedReq.SystemPrompt != "system" {
		t.Errorf("expected system prompt 'system', got %q", receivedReq.SystemPrompt)
	}
}

func TestManager_CancelRequest(t *testing.T) {
	provider := NewMockProvider()
	provider.SetSupportsSession(false)

	// Block in SendMessage until context is canceled
	sendStarted := make(chan struct{})
	provider.SetSendMessageFunc(func(ctx context.Context, req Request, onChunk func(StreamEvent)) error {
		close(sendStarted)
		<-ctx.Done()
		return ctx.Err()
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")

	// Start message in background
	go manager.SendMessage(sessionID, "hello", "", "", "", nil, nil, 60)

	// Wait for send to start
	<-sendStarted

	// Cancel the request
	manager.CancelRequest(sessionID)

	// Wait for cancellation to propagate
	time.Sleep(50 * time.Millisecond)

	// Verify done event was sent
	events := emitter.getEvents()
	hasDone := false
	for _, e := range events {
		if event, ok := e.Data.(AIResponseEvent); ok && event.Done {
			hasDone = true
			break
		}
	}
	if !hasDone {
		t.Error("expected done event after cancel")
	}
}

func TestManager_ClearSession(t *testing.T) {
	provider := NewMockProvider()
	manager := NewManager(provider)

	sessionID := manager.StartSession("")
	newSessionID := manager.ClearSession(sessionID)

	if newSessionID == "" {
		t.Fatal("expected new session ID")
	}
	if newSessionID == sessionID {
		t.Fatal("expected different session ID after clear")
	}
}

func TestManager_CloseSession(t *testing.T) {
	provider := NewMockProvider()
	var capturedSession *MockSession
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		capturedSession = NewMockSession(onEvent)
		return capturedSession, nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")
	manager.SendMessage(sessionID, "hello", "", "", "", nil, nil, 0)

	// Wait for session to start
	time.Sleep(50 * time.Millisecond)

	// Close the session
	manager.CloseSession(sessionID)

	// Verify CLI session was closed
	if capturedSession != nil && !capturedSession.IsClosed() {
		t.Error("expected CLI session to be closed")
	}
}

func TestManager_OnClientDisconnect(t *testing.T) {
	provider := NewMockProvider()
	var sessions []*MockSession
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		sess := NewMockSession(onEvent)
		sessions = append(sessions, sess)
		return sess, nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	// Start sessions for two clients
	clientA := "client-A"
	clientB := "client-B"

	sessionA1 := manager.StartSession(clientA)
	sessionA2 := manager.StartSession(clientA)
	sessionB := manager.StartSession(clientB)

	// Start messages to create CLI sessions
	manager.SendMessage(sessionA1, "hello", "", "", "", nil, nil, 0)
	manager.SendMessage(sessionA2, "hello", "", "", "", nil, nil, 0)
	manager.SendMessage(sessionB, "hello", "", "", "", nil, nil, 0)

	// Wait for sessions to start
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	// Disconnect client A
	manager.OnClientDisconnect(clientA)

	// Sessions for client A should be closed
	if !sessions[0].IsClosed() {
		t.Error("expected session A1 to be closed")
	}
	if !sessions[1].IsClosed() {
		t.Error("expected session A2 to be closed")
	}

	// Session for client B should still be open
	if sessions[2].IsClosed() {
		t.Error("expected session B to still be open")
	}
}

func TestManager_CloseAllSessions(t *testing.T) {
	provider := NewMockProvider()
	var sessions []*MockSession
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		sess := NewMockSession(onEvent)
		sessions = append(sessions, sess)
		return sess, nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	// Start multiple sessions
	session1 := manager.StartSession("")
	session2 := manager.StartSession("")

	manager.SendMessage(session1, "hello", "", "", "", nil, nil, 0)
	manager.SendMessage(session2, "hello", "", "", "", nil, nil, 0)

	// Wait for sessions to start
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Close all sessions
	manager.CloseAllSessions()

	// All sessions should be closed
	for i, sess := range sessions {
		if !sess.IsClosed() {
			t.Errorf("expected session %d to be closed", i)
		}
	}
}

func TestManager_EventRouting(t *testing.T) {
	provider := NewMockProvider()

	var sessionEmitFunc func(StreamEvent)
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		sessionEmitFunc = onEvent
		return NewMockSession(onEvent), nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")
	manager.SendMessage(sessionID, "hello", "", "", "", nil, nil, 0)

	// Wait for session to start
	time.Sleep(50 * time.Millisecond)

	// Emit events through the session
	sessionEmitFunc(StreamEvent{Type: "text", Content: "chunk1"})
	sessionEmitFunc(StreamEvent{Type: "text", Content: "chunk2"})
	sessionEmitFunc(StreamEvent{Type: "done"})

	// Wait for events to propagate
	time.Sleep(50 * time.Millisecond)

	// Check emitted events
	events := emitter.getEvents()
	textChunks := 0
	hasDone := false
	for _, e := range events {
		event, ok := e.Data.(AIResponseEvent)
		if !ok {
			continue
		}
		if event.Chunk != "" {
			textChunks++
		}
		if event.Done {
			hasDone = true
		}
	}

	if textChunks < 2 {
		t.Errorf("expected at least 2 text chunks, got %d", textChunks)
	}
	if !hasDone {
		t.Error("expected done event")
	}
}

func TestManager_SessionRestartsOnToolChange(t *testing.T) {
	provider := NewMockProvider()
	var sessions []*MockSession
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		sess := NewMockSession(onEvent)
		sessions = append(sessions, sess)
		return sess, nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")

	// First message with initial tools
	manager.SendMessage(sessionID, "hello", "", "", "", []string{"tool1"}, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Second message with same tools — should reuse session
	manager.SendMessage(sessionID, "hello2", "", "", "", []string{"tool1"}, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 1 {
		t.Fatalf("expected still 1 session (reused), got %d", len(sessions))
	}

	// Third message with DIFFERENT tools — should restart session
	manager.SendMessage(sessionID, "hello3", "", "", "", []string{"tool1", "tool2"}, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions (restarted), got %d", len(sessions))
	}

	// Old session should be closed
	if !sessions[0].IsClosed() {
		t.Error("expected old session to be closed after tool change")
	}
}

func TestManager_DeadSessionRecovery(t *testing.T) {
	provider := NewMockProvider()
	var sessions []*MockSession
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		sess := NewMockSession(onEvent)
		sessions = append(sessions, sess)
		return sess, nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")

	// First message starts a session
	manager.SendMessage(sessionID, "hello", "", "", "", []string{"tool1"}, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Simulate the CLI session dying (e.g., process crash)
	sessions[0].Close()

	// Next message should detect dead session and start a new one
	manager.SendMessage(sessionID, "hello2", "", "", "", []string{"tool1"}, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions (recovered from dead), got %d", len(sessions))
	}
}

func TestManager_StaleEventsFilteredAfterRestart(t *testing.T) {
	provider := NewMockProvider()
	var sessionEmitFuncs []func(StreamEvent)
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		sessionEmitFuncs = append(sessionEmitFuncs, onEvent)
		return NewMockSession(onEvent), nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")

	// First message starts session
	manager.SendMessage(sessionID, "hello", "", "", "", []string{"tool1"}, nil, 0)
	time.Sleep(50 * time.Millisecond)

	// Trigger session restart via tool change
	manager.SendMessage(sessionID, "hello2", "", "", "", []string{"tool1", "tool2"}, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessionEmitFuncs) != 2 {
		t.Fatalf("expected 2 emit funcs, got %d", len(sessionEmitFuncs))
	}

	// Clear events
	emitter.mu.Lock()
	emitter.events = nil
	emitter.mu.Unlock()

	// Emit error from OLD session's emitFunc (simulates waitLoop race)
	sessionEmitFuncs[0](StreamEvent{Type: "error", Content: "stale error from old process"})
	time.Sleep(10 * time.Millisecond)

	// Stale event should be filtered — no error events
	events := emitter.getEvents()
	for _, e := range events {
		if event, ok := e.Data.(AIResponseEvent); ok && event.Error != "" {
			t.Errorf("expected stale event to be filtered, got error: %q", event.Error)
		}
	}
}

func TestParseModel(t *testing.T) {
	tests := []struct {
		input     string
		wantProv  string
		wantModel string
	}{
		{"claude-cli/sonnet", "claude-cli", "sonnet"},
		{"codex-cli/o3", "codex-cli", "o3"},
		{"sonnet", "", "sonnet"},
		{"", "", ""},
		{"provider/model/extra", "provider", "model/extra"},
	}

	for _, tc := range tests {
		prov, model := parseModel(tc.input)
		if prov != tc.wantProv || model != tc.wantModel {
			t.Errorf("parseModel(%q) = (%q, %q), want (%q, %q)",
				tc.input, prov, model, tc.wantProv, tc.wantModel)
		}
	}
}

func TestManager_SessionRestartsOnModelChange(t *testing.T) {
	provider := NewMockProvider()
	var sessions []*MockSession
	provider.SetStartSessionFunc(func(sessionID, systemPrompt, model, k8sContext string, allowedTools, allowedCommands []string, onEvent func(StreamEvent)) (Session, error) {
		sess := NewMockSession(onEvent)
		sessions = append(sessions, sess)
		return sess, nil
	})

	manager := NewManager(provider)

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")

	// First message with model "sonnet"
	manager.SendMessage(sessionID, "hello", "", "sonnet", "", nil, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Second message with same model — should reuse session
	manager.SendMessage(sessionID, "hello2", "", "sonnet", "", nil, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 1 {
		t.Fatalf("expected still 1 session (reused), got %d", len(sessions))
	}

	// Third message with DIFFERENT model — should restart session
	manager.SendMessage(sessionID, "hello3", "", "opus", "", nil, nil, 0)
	time.Sleep(50 * time.Millisecond)

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions (model change restart), got %d", len(sessions))
	}

	// Old session should be closed
	if !sessions[0].IsClosed() {
		t.Error("expected old session to be closed after model change")
	}
}

func TestManager_ProviderSwitch(t *testing.T) {
	// Create a registry with two mock providers
	registry := NewRegistry()

	providerA := NewMockProvider()
	providerA.SetName("Provider A")
	providerA.SetAvailable(true, "available")

	providerB := NewMockProvider()
	providerB.SetName("Provider B")
	providerB.SetAvailable(true, "available")
	providerB.SetSupportsSession(false) // one-shot mode for provider B

	var providerARef, providerBRef *MockProvider
	registry.Register("prov-a", func() Provider {
		providerARef = NewMockProvider()
		providerARef.SetName("Provider A")
		providerARef.SetAvailable(true, "available")
		providerARef.SetSupportsSession(true)
		return providerARef
	}, nil)
	registry.Register("prov-b", func() Provider {
		providerBRef = NewMockProvider()
		providerBRef.SetName("Provider B")
		providerBRef.SetAvailable(true, "available")
		providerBRef.SetSupportsSession(false)
		return providerBRef
	}, nil)

	manager, err := NewManagerWithRegistry(registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")

	// Send with compound model for prov-b
	manager.SendMessage(sessionID, "hello", "", "prov-b/mymodel", "", nil, nil, 0)
	time.Sleep(50 * time.Millisecond)

	// Provider should have switched
	if manager.ProviderID() != "prov-b" {
		t.Errorf("expected providerID 'prov-b', got %q", manager.ProviderID())
	}

	// Provider B doesn't support sessions, so it uses one-shot mode
	if providerBRef == nil {
		t.Fatal("expected provider B to be created")
	}
	calls := providerBRef.GetSendMessageCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 send call to provider B, got %d", len(calls))
	}
	if calls[0].Model != "mymodel" {
		t.Errorf("expected model 'mymodel' passed to provider, got %q", calls[0].Model)
	}
}

func TestManager_ProviderSwitch_InvalidProvider(t *testing.T) {
	registry := NewRegistry()
	registry.Register("prov-a", func() Provider {
		p := NewMockProvider()
		p.SetAvailable(true, "ok")
		return p
	}, nil)

	manager, err := NewManagerWithRegistry(registry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	emitter := &mockEmitter{}
	ctx := context.Background()
	manager.SetEmitter(emitter, ctx)

	sessionID := manager.StartSession("")

	// Send with unknown provider
	result := manager.SendMessage(sessionID, "hello", "", "unknown/model", "", nil, nil, 0)
	if result {
		t.Error("expected SendMessage to return false for unknown provider")
	}

	// Should have emitted error
	time.Sleep(10 * time.Millisecond)
	events := emitter.getEvents()
	found := false
	for _, e := range events {
		if event, ok := e.Data.(AIResponseEvent); ok && event.Error != "" {
			found = true
			if event.Error != fmt.Sprintf("provider %q not available: provider %q not registered", "unknown", "unknown") {
				t.Errorf("unexpected error: %q", event.Error)
			}
		}
	}
	if !found {
		t.Error("expected error event for unknown provider")
	}
}
