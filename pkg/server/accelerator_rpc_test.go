package server

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/agent"
)

type recordingAcceleratorRPCCaller struct {
	mu      sync.Mutex
	calls   []acceleratorRPCCall
	entered chan acceleratorRPCCall
	release <-chan struct{}
	result  interface{}
	err     error
}

type acceleratorRPCCall struct {
	context agent.AuthenticatedCallContext
	method  string
	args    []json.RawMessage
}

func (c *recordingAcceleratorRPCCaller) CallMethod(call agent.AuthenticatedCallContext, method string, args []json.RawMessage) (interface{}, error) {
	record := acceleratorRPCCall{context: call, method: method, args: append([]json.RawMessage(nil), args...)}
	c.mu.Lock()
	c.calls = append(c.calls, record)
	c.mu.Unlock()
	if c.entered != nil {
		c.entered <- record
	}
	if c.release != nil {
		<-c.release
	}
	return c.result, c.err
}

func (c *recordingAcceleratorRPCCaller) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func TestCreatorRPCDispatchExactPolicyAndArity(t *testing.T) {
	callContext := agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "session"}
	caller := &recordingAcceleratorRPCCaller{result: []string{"safe"}}
	authorizer := MethodAuthorizerFunc(func(call agent.AuthenticatedCallContext, method string) bool {
		_, known := agent.LookupMethodPolicy(method)
		return call == callContext && known
	})
	dispatcher := NewAcceleratorRPCDispatcher(caller, authorizer)
	results := make(chan []byte, 16)
	connection := dispatcher.attach(AcceleratorSessionSnapshot{CallContext: callContext, Generation: 7}, func(payload []byte) bool {
		results <- append([]byte(nil), payload...)
		return true
	}, make(chan struct{}))
	if connection == nil {
		t.Fatal("authenticated creator dispatcher did not attach")
	}
	nonce := "AAAAAAAAAAAAAAAAAAAAAA"
	encoders := []func(string) ([]byte, error){
		func(id string) ([]byte, error) {
			return acceleratorsecret.EncodeListSecretsMetadataCall(id, "request", "ns", true)
		},
		func(id string) ([]byte, error) { return acceleratorsecret.EncodeGetSecretDataCall(id, "ns", "name") },
		func(id string) ([]byte, error) { return acceleratorsecret.EncodeGetSecretYAMLCall(id, "ns", "name") },
		func(id string) ([]byte, error) { return acceleratorsecret.EncodeCancelListRequestCall(id, "request") },
		func(id string) ([]byte, error) {
			return acceleratorsecret.EncodeSubscribeSecretWatcherCall(id, "ns", true)
		},
		func(id string) ([]byte, error) {
			return acceleratorsecret.EncodeUnsubscribeSecretWatcherCall(id, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		},
	}
	for index, encode := range encoders {
		id, _ := acceleratorsecret.CallID(nonce, uint64(index+1))
		payload, err := encode(id)
		if err != nil || !connection.handle(payload) {
			t.Fatalf("call %d handle=%v err=%v", index, err == nil, err)
		}
		select {
		case raw := <-results:
			result, err := acceleratorsecret.DecodeResult(raw)
			if err != nil || result.ID != id || result.Status != acceleratorsecret.ResultStatusOK {
				t.Fatalf("call %d result=%#v err=%v", index, result, err)
			}
		case <-time.After(time.Second):
			t.Fatal("creator RPC result timed out")
		}
	}
	if caller.count() != 6 {
		t.Fatalf("caller count=%d", caller.count())
	}
	if dispatcher.attach(AcceleratorSessionSnapshot{Generation: 1}, func([]byte) bool { return true }, make(chan struct{})) != nil {
		t.Fatal("partial trusted context attached")
	}
	deniedCaller := &recordingAcceleratorRPCCaller{}
	denied := NewAcceleratorRPCDispatcher(deniedCaller, MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool { return false }))
	deniedResults := make(chan []byte, 1)
	deniedConn := denied.attach(AcceleratorSessionSnapshot{CallContext: callContext, Generation: 8}, func(payload []byte) bool { deniedResults <- payload; return true }, make(chan struct{}))
	id, _ := acceleratorsecret.CallID(nonce, 1)
	payload, _ := acceleratorsecret.EncodeGetSecretDataCall(id, "ns", "name")
	if !deniedConn.handle(payload) {
		t.Fatal("policy denial incorrectly failed the transport")
	}
	result, err := acceleratorsecret.DecodeResult(<-deniedResults)
	if err != nil || result.Reason != acceleratorsecret.ReasonForbidden || deniedCaller.count() != 0 {
		t.Fatalf("denial=%#v err=%v calls=%d", result, err, deniedCaller.count())
	}
	for _, invalid := range [][]byte{
		[]byte(`{"type":"call","id":"` + id + `","operation":"GetSecretData","args":["ns"]}`),
		[]byte(`{"type":"call","id":"` + id + `","operation":"CreateSecret","args":[]}`),
	} {
		if deniedConn.handle(invalid) {
			t.Fatalf("invalid call did not fail closed: %s", invalid)
		}
	}
}

func TestCreatorRPCOutOfOrderCapacityAndGeneration(t *testing.T) {
	callContext := agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "session"}
	release := make(chan struct{})
	caller := &recordingAcceleratorRPCCaller{entered: make(chan acceleratorRPCCall, acceleratorsecret.MaxConcurrentCalls), release: release, result: true}
	dispatcher := NewAcceleratorRPCDispatcher(caller, MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool { return true }))
	results := make(chan []byte, acceleratorsecret.MaxConcurrentCalls+2)
	terminal := make(chan struct{})
	connection := dispatcher.attach(AcceleratorSessionSnapshot{CallContext: callContext, Generation: 3}, func(payload []byte) bool {
		results <- append([]byte(nil), payload...)
		return true
	}, terminal)
	nonce := "AAAAAAAAAAAAAAAAAAAAAA"
	for index := 1; index <= acceleratorsecret.MaxConcurrentCalls; index++ {
		id, _ := acceleratorsecret.CallID(nonce, uint64(index))
		payload, _ := acceleratorsecret.EncodeCancelListRequestCall(id, "remote")
		if !connection.handle(payload) {
			t.Fatalf("call %d rejected", index)
		}
	}
	for index := 0; index < acceleratorsecret.MaxConcurrentCalls; index++ {
		<-caller.entered
	}
	ninth, _ := acceleratorsecret.CallID(nonce, 9)
	ninthPayload, _ := acceleratorsecret.EncodeCancelListRequestCall(ninth, "remote")
	if !connection.handle(ninthPayload) {
		t.Fatal("capacity result closed transport")
	}
	capacity, err := acceleratorsecret.DecodeResult(<-results)
	if err != nil || capacity.ID != ninth || capacity.Reason != acceleratorsecret.ReasonCapacity {
		t.Fatalf("capacity=%#v err=%v", capacity, err)
	}
	close(release)
	seen := make(map[string]struct{})
	for len(seen) < acceleratorsecret.MaxConcurrentCalls {
		select {
		case raw := <-results:
			result, err := acceleratorsecret.DecodeResult(raw)
			if err != nil || result.Status != acceleratorsecret.ResultStatusOK {
				t.Fatalf("worker result=%#v err=%v", result, err)
			}
			seen[result.ID] = struct{}{}
		case <-time.After(time.Second):
			t.Fatal("worker results timed out")
		}
	}
	if connection.handle(ninthPayload) {
		t.Fatal("replayed/lower call ID remained open")
	}

	lateRelease := make(chan struct{})
	lateCaller := &recordingAcceleratorRPCCaller{entered: make(chan acceleratorRPCCall, 1), release: lateRelease, result: "sensitive-result"}
	lateDispatcher := NewAcceleratorRPCDispatcher(lateCaller, MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool { return true }))
	lateResults := make(chan []byte, 1)
	oldTerminal := make(chan struct{})
	old := lateDispatcher.attach(AcceleratorSessionSnapshot{CallContext: callContext, Generation: 4}, func(payload []byte) bool { lateResults <- payload; return true }, oldTerminal)
	id, _ := acceleratorsecret.CallID("AAAAAAAAAAAAAAAAAAAAAA", 1)
	payload, _ := acceleratorsecret.EncodeGetSecretYAMLCall(id, "ns", "name")
	if !old.handle(payload) {
		t.Fatal("late call rejected")
	}
	<-lateCaller.entered
	close(oldTerminal)
	close(lateRelease)
	old.waitWorkers()
	select {
	case raw := <-lateResults:
		t.Fatalf("old generation received late result: %s", raw)
	default:
	}
	if !errors.Is(old.err(), errAcceleratorRPCUnavailable) {
		t.Fatalf("old connection error=%v", old.err())
	}
}

func TestCreatorRPCLogsProxiedCallLifecycle(t *testing.T) {
	logger := &recordingAcceleratorLogger{}
	callContext := agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "logged-session"}
	dispatcher := NewAcceleratorRPCDispatcher(&recordingAcceleratorRPCCaller{result: "yaml"}, MethodAuthorizerFunc(func(agent.AuthenticatedCallContext, string) bool { return true }))
	results := make(chan []byte, 1)
	connection := dispatcher.attachWithLogger(AcceleratorSessionSnapshot{CallContext: callContext, Generation: 4}, func(payload []byte) bool {
		results <- append([]byte(nil), payload...)
		return true
	}, make(chan struct{}), logger.logf)
	id, _ := acceleratorsecret.CallID("AAAAAAAAAAAAAAAAAAAAAA", 1)
	payload, _ := acceleratorsecret.EncodeGetSecretYAMLCall(id, "ns", "name")
	if !connection.handle(payload) {
		t.Fatal("proxied call was rejected")
	}
	select {
	case <-results:
	case <-time.After(time.Second):
		t.Fatal("proxied result timed out")
	}
	connection.waitWorkers()

	got := logger.text()
	if !strings.Contains(got, "Accelerator proxy call started session=\"logged-session\" generation=4 operation=\"GetSecretYaml\"") || !strings.Contains(got, "Accelerator proxy call completed session=\"logged-session\" generation=4 operation=\"GetSecretYaml\" outcome=\"ok\"") {
		t.Fatalf("proxy logs=%q", got)
	}
}
