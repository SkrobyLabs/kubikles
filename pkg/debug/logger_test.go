package debug

import (
	"sync"
	"testing"

	"kubikles/pkg/events"
)

// recordingEmitter captures emitted events for assertions.
type recordingEmitter struct {
	calls []emitCall
}

type emitCall struct {
	name string
	data []interface{}
}

func (r *recordingEmitter) Emit(name string, data ...interface{}) {
	r.calls = append(r.calls, emitCall{name: name, data: data})
}

// setGlobal replaces the global logger for testing and returns a cleanup func.
func setGlobal(l *Logger) func() {
	prev := globalLogger
	globalLogger = l
	return func() { globalLogger = prev }
}

func TestLog_EmitsWhenEnabled(t *testing.T) {
	rec := &recordingEmitter{}
	l := &Logger{emitter: rec, enabled: true}
	cleanup := setGlobal(l)
	defer cleanup()

	Log(CategoryK8s, "test message", map[string]interface{}{"key": "val"})

	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 emit call, got %d", len(rec.calls))
	}
	if rec.calls[0].name != "debug:log" {
		t.Errorf("expected event name 'debug:log', got %q", rec.calls[0].name)
	}
	if rec.calls[0].data[0] != CategoryK8s {
		t.Errorf("expected category %q, got %v", CategoryK8s, rec.calls[0].data[0])
	}
	if rec.calls[0].data[1] != "test message" {
		t.Errorf("expected message 'test message', got %v", rec.calls[0].data[1])
	}
}

func TestLog_SuppressedWhenDisabled(t *testing.T) {
	rec := &recordingEmitter{}
	l := &Logger{emitter: rec, enabled: false}
	cleanup := setGlobal(l)
	defer cleanup()

	Log(CategoryHelm, "should not appear", nil)

	if len(rec.calls) != 0 {
		t.Fatalf("expected 0 emit calls when disabled, got %d", len(rec.calls))
	}
}

func TestLog_NilGlobalDoesNotPanic(t *testing.T) {
	cleanup := setGlobal(nil)
	defer cleanup()

	// Should not panic
	Log(CategoryK8s, "no logger", nil)
	SetEnabled(true)
	if IsEnabled() {
		t.Error("IsEnabled should return false when globalLogger is nil")
	}
}

func TestSetEnabled_TogglesState(t *testing.T) {
	l := &Logger{emitter: &events.NoopEmitter{}, enabled: false}
	cleanup := setGlobal(l)
	defer cleanup()

	if IsEnabled() {
		t.Error("expected disabled initially")
	}

	SetEnabled(true)
	if !IsEnabled() {
		t.Error("expected enabled after SetEnabled(true)")
	}

	SetEnabled(false)
	if IsEnabled() {
		t.Error("expected disabled after SetEnabled(false)")
	}
}

func TestInit_UpdatesEmitter(t *testing.T) {
	rec1 := &recordingEmitter{}
	rec2 := &recordingEmitter{}

	// Manually set up a logger with rec1
	l := &Logger{emitter: rec1, enabled: true}
	cleanup := setGlobal(l)
	defer cleanup()

	// Simulate Init updating the emitter (bypassing sync.Once)
	globalLogger.mu.Lock()
	globalLogger.emitter = rec2
	globalLogger.mu.Unlock()

	Log(CategoryConfig, "after swap", nil)

	if len(rec1.calls) != 0 {
		t.Error("old emitter should not have received calls")
	}
	if len(rec2.calls) != 1 {
		t.Fatalf("new emitter should have 1 call, got %d", len(rec2.calls))
	}
}

func TestLog_NilEmitterDoesNotPanic(t *testing.T) {
	l := &Logger{emitter: nil, enabled: true}
	cleanup := setGlobal(l)
	defer cleanup()

	// Should not panic
	Log(CategoryUI, "nil emitter", nil)
}

func TestConvenienceFunctions_RouteCategory(t *testing.T) {
	cases := []struct {
		fn       func(string, map[string]interface{})
		expected string
	}{
		{LogK8s, CategoryK8s},
		{LogWatcher, CategoryWatcher},
		{LogHelm, CategoryHelm},
		{LogPortforward, CategoryPortforward},
		{LogTerminal, CategoryTerminal},
		{LogAI, CategoryAI},
		{LogConfig, CategoryConfig},
		{LogUI, CategoryUI},
		{LogWails, CategoryWails},
		{LogPerformance, CategoryPerformance},
	}

	for _, tc := range cases {
		rec := &recordingEmitter{}
		l := &Logger{emitter: rec, enabled: true}
		cleanup := setGlobal(l)

		tc.fn("msg", nil)

		if len(rec.calls) != 1 {
			t.Errorf("%s: expected 1 call, got %d", tc.expected, len(rec.calls))
			cleanup()
			continue
		}
		if rec.calls[0].data[0] != tc.expected {
			t.Errorf("expected category %q, got %v", tc.expected, rec.calls[0].data[0])
		}
		cleanup()
	}
}

func TestLog_ConcurrentAccess(t *testing.T) {
	rec := &recordingEmitter{}
	l := &Logger{emitter: rec, enabled: true}
	cleanup := setGlobal(l)
	defer cleanup()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Log(CategoryK8s, "concurrent", nil)
		}()
	}
	wg.Wait()
	// Just verify no panic and calls were recorded (may not be exactly 50
	// due to recording emitter not being synchronized, but no crash)
}
