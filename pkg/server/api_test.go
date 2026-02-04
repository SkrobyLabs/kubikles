package server

import (
	"encoding/json"
	"testing"
)

// TestStruct is a test struct for method calling tests
type TestStruct struct{}

func (t *TestStruct) NoArgs() string {
	return "no args"
}

func (t *TestStruct) SingleArg(s string) string {
	return "got: " + s
}

func (t *TestStruct) MultipleArgs(a, b string) string {
	return a + " " + b
}

func (t *TestStruct) WithError(fail bool) (string, error) {
	if fail {
		return "", &testError{"forced failure"}
	}
	return "success", nil
}

func (t *TestStruct) IntArg(n int) int {
	return n * 2
}

func (t *TestStruct) StructArg(input testInput) string {
	return input.Name + ": " + input.Value
}

func (t *TestStruct) SliceArg(items []string) int {
	return len(items)
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

type testInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func TestReflectMethodCaller_NoArgs(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	result, err := caller.CallMethod("NoArgs", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "no args" {
		t.Errorf("expected 'no args', got %v", result)
	}
}

func TestReflectMethodCaller_SingleArg(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	args := []json.RawMessage{json.RawMessage(`"hello"`)}
	result, err := caller.CallMethod("SingleArg", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "got: hello" {
		t.Errorf("expected 'got: hello', got %v", result)
	}
}

func TestReflectMethodCaller_MultipleArgs(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	args := []json.RawMessage{
		json.RawMessage(`"foo"`),
		json.RawMessage(`"bar"`),
	}
	result, err := caller.CallMethod("MultipleArgs", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "foo bar" {
		t.Errorf("expected 'foo bar', got %v", result)
	}
}

func TestReflectMethodCaller_WithError_Success(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	args := []json.RawMessage{json.RawMessage(`false`)}
	result, err := caller.CallMethod("WithError", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %v", result)
	}
}

func TestReflectMethodCaller_WithError_Failure(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	args := []json.RawMessage{json.RawMessage(`true`)}
	_, err := caller.CallMethod("WithError", args)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "forced failure" {
		t.Errorf("expected 'forced failure', got %v", err.Error())
	}
}

func TestReflectMethodCaller_IntArg(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	args := []json.RawMessage{json.RawMessage(`21`)}
	result, err := caller.CallMethod("IntArg", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %v", result)
	}
}

func TestReflectMethodCaller_StructArg(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	args := []json.RawMessage{json.RawMessage(`{"name":"test","value":"123"}`)}
	result, err := caller.CallMethod("StructArg", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "test: 123" {
		t.Errorf("expected 'test: 123', got %v", result)
	}
}

func TestReflectMethodCaller_SliceArg(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	args := []json.RawMessage{json.RawMessage(`["a","b","c"]`)}
	result, err := caller.CallMethod("SliceArg", args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 3 {
		t.Errorf("expected 3, got %v", result)
	}
}

func TestReflectMethodCaller_MethodNotFound(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	_, err := caller.CallMethod("NonExistent", nil)
	if err == nil {
		t.Fatal("expected error for non-existent method")
	}
}

func TestReflectMethodCaller_HasMethod(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	if !caller.HasMethod("NoArgs") {
		t.Error("expected HasMethod('NoArgs') to be true")
	}
	if caller.HasMethod("NonExistent") {
		t.Error("expected HasMethod('NonExistent') to be false")
	}
}

func TestReflectMethodCaller_ListMethods(t *testing.T) {
	caller := NewReflectMethodCaller(&TestStruct{})

	methods := caller.ListMethods()
	expected := map[string]bool{
		"NoArgs":       true,
		"SingleArg":    true,
		"MultipleArgs": true,
		"WithError":    true,
		"IntArg":       true,
		"StructArg":    true,
		"SliceArg":     true,
	}

	for _, m := range methods {
		if !expected[m] {
			t.Errorf("unexpected method: %s", m)
		}
		delete(expected, m)
	}

	for m := range expected {
		t.Errorf("missing method: %s", m)
	}
}
