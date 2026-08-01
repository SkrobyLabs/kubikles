package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kubikles/pkg/agent"
)

type recordingMethodCaller struct {
	context agent.AuthenticatedCallContext
	method  string
	args    []json.RawMessage
}

func (c *recordingMethodCaller) CallMethod(context agent.AuthenticatedCallContext, method string, args []json.RawMessage) (interface{}, error) {
	c.context = context
	c.method = method
	c.args = args
	return "ok", nil
}

func TestHandleAPIPassesLocalCallContext(t *testing.T) {
	caller := &recordingMethodCaller{}
	server := &Server{caller: caller}
	body := []byte(`{"method":"Example","args":[{"PrincipalID":"forged","SessionID":"forged"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/api/call", bytes.NewReader(body))
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
	var envelope map[string]interface{}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope["data"] != "ok" {
		t.Errorf("response = %#v, want data envelope", envelope)
	}
}
