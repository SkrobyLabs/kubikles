package server

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"kubikles/pkg/agent"
)

type recordingMethodCaller struct {
	mu      sync.Mutex
	context agent.AuthenticatedCallContext
	method  string
	args    []json.RawMessage
	calls   int
	err     error
}

func (c *recordingMethodCaller) CallMethod(callContext agent.AuthenticatedCallContext, method string, args []json.RawMessage) (interface{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.context, c.method, c.args = callContext, method, args
	c.calls++
	return "ok", c.err
}

func (c *recordingMethodCaller) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestHandleAPIPassesLocalCallContext(t *testing.T) {
	caller := &recordingMethodCaller{}
	server := &Server{caller: caller}
	body := []byte(`{"method":"Example","args":[{"PrincipalID":"forged","SessionID":"forged"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/call", strings.NewReader(string(body)))
	response := httptest.NewRecorder()

	server.handleAPI(response, request)

	if caller.context != agent.LocalCallContext() {
		t.Fatalf("context = %#v, want local zero context", caller.context)
	}
	if caller.method != "Example" || len(caller.args) != 1 {
		t.Fatalf("call = %q %#v, want Example and one argument", caller.method, caller.args)
	}
	if got := response.Code; got != http.StatusOK {
		t.Fatalf("status = %d, want %d", got, http.StatusOK)
	}
}

func TestServerOptionsValidateAcceleratorListenAddress(t *testing.T) {
	guard := ProtectedRouteGuard(func(next http.Handler) http.Handler { return next })
	for _, address := range []string{"127.0.0.1:0", "127.0.0.1:8080"} {
		options := Options{ListenAddress: address, BoundaryMode: BoundaryModeAccelerator, ProtectedRouteGuard: guard}
		if err := validateOptions(options); err != nil {
			t.Errorf("validateOptions(%q) error = %v", address, err)
		}
	}
	for _, address := range []string{"", ":8080", "localhost:8080", "0.0.0.0:8080", "[::1]:8080", "192.0.2.1:8080", "127.0.0.1", "127.0.0.1:-1", "127.0.0.1:65536", "127.0.0.1:notaport"} {
		t.Run(address, func(t *testing.T) {
			options := Options{ListenAddress: address, BoundaryMode: BoundaryModeAccelerator, ProtectedRouteGuard: guard}
			if err := validateOptions(options); !errors.Is(err, ErrInvalidListenAddress) {
				t.Fatalf("error = %v, want ErrInvalidListenAddress", err)
			}
		})
	}
	if err := validateOptions(Options{ListenAddress: "127.0.0.1:8080", BoundaryMode: BoundaryModeAccelerator}); !errors.Is(err, ErrProtectedRouteGuardRequired) {
		t.Fatalf("nil guard error = %v", err)
	}
}

func TestEmitEventRoutesOnlyInAcceleratorMode(t *testing.T) {
	registry := newAcceleratorSessionRegistry("instance", nil, acceleratorTestConfig())
	conn := newFakeAcceleratorConn()
	socket, ok := registry.register(acceleratorTestCall("route"), conn)
	if !ok {
		t.Fatal("register rejected")
	}
	defer closeAcceleratorSocket(t, socket)

	compatibilityOptions := CompatibilityOptions(0, nil)
	compatibilityOptions.AcceleratorSessions = registry
	compatibility := newServer(nil, embed.FS{}, compatibilityOptions)
	compatibility.EmitEvent("ordinary", "broadcast")
	select {
	case event := <-compatibility.broadcast:
		if event.Name != "ordinary" {
			t.Fatalf("compatibility event=%+v", event)
		}
	default:
		t.Fatal("compatibility event was not broadcast")
	}
	if conn.writeCount() != 1 {
		t.Fatalf("compatibility event reached Accelerator registry: writes=%d", conn.writeCount())
	}

	acceleratorOptions := AcceleratorOptions(0, nil, DenyProtectedRoutes)
	acceleratorOptions.AcceleratorSessions = registry
	accelerator := newServer(nil, embed.FS{}, acceleratorOptions)
	accelerator.EmitEvent("accelerated", "registry")
	waitAccelerator(t, "Accelerator registry event", func() bool { return conn.writeCount() == 2 })
	select {
	case event := <-accelerator.broadcast:
		t.Fatalf("Accelerator event entered compatibility broadcast: %+v", event)
	default:
	}
}

func TestCompatibilityOptionsPreserveWildcardBind(t *testing.T) {
	if got := CompatibilityOptions(8080, nil).ListenAddress; got != ":8080" {
		t.Fatalf("ListenAddress = %q", got)
	}
}

type failingListener struct{ err error }

func (l failingListener) Accept() (net.Conn, error) { return nil, l.err }
func (failingListener) Close() error                { return nil }
func (failingListener) Addr() net.Addr              { return testAddr("failing") }

type testAddr string

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return string(a) }

func TestListenReturnsBindError(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(port, nil, DenyProtectedRoutes))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Listen(); err == nil {
		t.Fatal("expected occupied-port error")
	}
}

func TestServeReturnsUnexpectedListenerError(t *testing.T) {
	want := errors.New("accept failed")
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(failingListener{err: want}); !errors.Is(err, want) {
		t.Fatalf("Serve() error = %v", err)
	}
}

func TestRunReturnsServeError(t *testing.T) {
	want := errors.New("run accept failed")
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
	if err != nil {
		t.Fatal(err)
	}
	server.listenFunc = func() (net.Listener, error) { return failingListener{err: want}, nil }
	if err := server.Run(context.Background()); !errors.Is(err, want) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunCancellationReturnsNil(t *testing.T) {
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := server.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunWithReadyListenerOrdering(t *testing.T) {
	t.Run("listen failure never calls ready", func(t *testing.T) {
		want := errors.New("bind failed")
		s, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
		if err != nil {
			t.Fatal(err)
		}
		s.listenFunc = func() (net.Listener, error) { return nil, want }
		calls := 0
		if err := s.RunWithReady(context.Background(), func() { calls++ }); !errors.Is(err, want) {
			t.Fatalf("RunWithReady error = %v", err)
		}
		if calls != 0 {
			t.Fatalf("ready calls after listen failure = %d", calls)
		}
	})

	t.Run("active listener calls ready once", func(t *testing.T) {
		s, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
		if err != nil {
			t.Fatal(err)
		}
		address := make(chan string, 1)
		s.listenFunc = func() (net.Listener, error) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err == nil {
				address <- listener.Addr().String()
			}
			return listener, err
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ready := make(chan struct{})
		var calls atomic.Int32
		result := make(chan error, 1)
		go func() {
			result <- s.RunWithReady(ctx, func() {
				calls.Add(1)
				close(ready)
			})
		}()
		var addr string
		select {
		case addr = <-address:
		case <-time.After(5 * time.Second):
			t.Fatal("listener was not created")
		}
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			t.Fatal("ready was not called")
		}
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatalf("listener did not accept after ready: %v", err)
		}
		_ = conn.Close()
		cancel()
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("RunWithReady cancellation = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("RunWithReady did not shut down")
		}
		if calls.Load() != 1 {
			t.Fatalf("ready calls = %d", calls.Load())
		}
	})

	t.Run("immediate serve failure is returned after successful listen", func(t *testing.T) {
		want := errors.New("immediate accept failure")
		s, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
		if err != nil {
			t.Fatal(err)
		}
		s.listenFunc = func() (net.Listener, error) { return failingListener{err: want}, nil }
		var calls atomic.Int32
		if err := s.RunWithReady(context.Background(), func() { calls.Add(1) }); !errors.Is(err, want) {
			t.Fatalf("RunWithReady immediate Serve error = %v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("successful-listen ready calls = %d", calls.Load())
		}
	})
}

func TestRunReturnsConcurrentCloseError(t *testing.T) {
	want := errors.New("concurrent listener close failed")
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
	if err != nil {
		t.Fatal(err)
	}
	listener := &closeErrorListener{closed: make(chan struct{}), accept: make(chan struct{}), err: want}
	server.listenFunc = func() (net.Listener, error) { return listener, nil }
	runResult := make(chan error, 1)
	go func() { runResult <- server.Run(context.Background()) }()
	<-listener.accept

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(ctx); !errors.Is(err, want) {
		t.Fatalf("Close() error = %v, want %v", err, want)
	}
	if err := <-runResult; !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
}

type closeErrorListener struct {
	closed chan struct{}
	accept chan struct{}
	seen   sync.Once
	once   sync.Once
	err    error
}

func (l *closeErrorListener) Accept() (net.Conn, error) {
	l.seen.Do(func() { close(l.accept) })
	<-l.closed
	return nil, net.ErrClosed
}

func (l *closeErrorListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.err
}

func (*closeErrorListener) Addr() net.Addr { return testAddr("close-error") }

func TestCloseAndRunReturnShutdownError(t *testing.T) {
	want := errors.New("listener close failed")
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
	if err != nil {
		t.Fatal(err)
	}
	listener := &closeErrorListener{closed: make(chan struct{}), accept: make(chan struct{}), err: want}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	<-listener.accept
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(ctx); !errors.Is(err, want) {
		t.Fatalf("Close() error = %v", err)
	}
	if err := <-serveResult; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	server, err = NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, nil, DenyProtectedRoutes))
	if err != nil {
		t.Fatal(err)
	}
	listener = &closeErrorListener{closed: make(chan struct{}), accept: make(chan struct{}), err: want}
	server.listenFunc = func() (net.Listener, error) { return listener, nil }
	runCtx, cancelRun := context.WithCancel(context.Background())
	runResult := make(chan error, 1)
	go func() { runResult <- server.Run(runCtx) }()
	<-listener.accept
	cancelRun()
	if err := <-runResult; !errors.Is(err, want) {
		t.Fatalf("Run() shutdown error = %v", err)
	}
}

func TestQuiesceAndCloseAreIdempotent(t *testing.T) {
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, AcceleratorOptions(0, ReadinessFunc(func(context.Context) error { return nil }), func(next http.Handler) http.Handler { return next }))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := server.Listen()
	if err != nil {
		t.Fatal(err)
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()

	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			server.Quiesce()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = server.Close(ctx)
			server.EmitEvent("after-close", nil)
		}()
	}
	group.Wait()
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not exit")
	}

	for path, want := range map[string]int{"/livez": http.StatusOK, "/readyz": http.StatusServiceUnavailable} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Host = "localhost"
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != want {
			t.Errorf("%s status = %d, want %d", path, response.Code, want)
		}
	}
}

func TestCloseDoesNotWaitForStalledCompatibilityWebSocketWrite(t *testing.T) {
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, CompatibilityOptions(0, nil))
	if err != nil {
		t.Fatal(err)
	}
	server.broadcast = make(chan Event)
	listener, err := server.Listen()
	if err != nil {
		t.Fatal(err)
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()

	wsURL := "ws://" + listener.Addr().String() + "/ws"
	connection, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var connected Event
	if err := connection.ReadJSON(&connected); err != nil {
		t.Fatalf("read connection event: %v", err)
	}

	server.clientsMu.RLock()
	clientCount := len(server.clients)
	if clientCount != 1 {
		server.clientsMu.RUnlock()
		t.Fatalf("clients = %d, want 1", clientCount)
	}
	var client *wsClient
	for client = range server.clients {
	}
	server.clientsMu.RUnlock()
	client.writeMu.Lock()
	defer client.writeMu.Unlock()

	emitted := make(chan struct{})
	go func() {
		server.EmitEvent("blocked", nil)
		close(emitted)
	}()
	select {
	case <-emitted:
	case <-time.After(time.Second):
		t.Fatal("broadcast was not accepted")
	}
	// Give the broadcaster a chance to enter the deliberately stalled WriteJSON call.
	for i := 0; i < 100; i++ {
		runtime.Gosched()
	}

	closeResult := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go func() { closeResult <- server.Close(ctx) }()
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() waited for stalled WebSocket write")
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("WebSocket remained open after Close")
	}
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not exit after Close")
	}
}

func TestCloseRejectsWebSocketUpgradedBeforeRegistration(t *testing.T) {
	server, err := NewWithOptions(&recordingMethodCaller{}, embed.FS{}, CompatibilityOptions(0, nil))
	if err != nil {
		t.Fatal(err)
	}
	upgraded := make(chan struct{})
	register := make(chan struct{})
	server.afterWebSocketUpgrade = func() {
		close(upgraded)
		<-register
	}
	listener, err := server.Listen()
	if err != nil {
		t.Fatal(err)
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()

	type dialResult struct {
		connection *websocket.Conn
		err        error
	}
	dialed := make(chan dialResult, 1)
	go func() {
		connection, _, err := websocket.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/ws", nil)
		dialed <- dialResult{connection: connection, err: err}
	}()
	select {
	case <-upgraded:
	case <-time.After(time.Second):
		t.Fatal("WebSocket was not upgraded")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	closeResult := make(chan error, 1)
	go func() { closeResult <- server.Close(ctx) }()
	select {
	case <-server.done:
	case <-time.After(time.Second):
		t.Fatal("Close did not begin shutdown")
	}
	close(register)

	result := <-dialed
	if result.err != nil {
		t.Fatalf("dial WebSocket: %v", result.err)
	}
	defer result.connection.Close()
	if err := result.connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := result.connection.ReadMessage(); err == nil {
		t.Fatal("WebSocket remained open after shutdown won registration race")
	}
	if err := <-closeResult; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	server.clientsMu.RLock()
	clientCount := len(server.clients)
	server.clientsMu.RUnlock()
	if clientCount != 0 {
		t.Fatalf("clients = %d, want 0", clientCount)
	}
	select {
	case err := <-serveResult:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not exit after Close")
	}
}
