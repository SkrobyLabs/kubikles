package main

import (
	"encoding/json"
	"testing"
)

// rawResource encodes a resource body the way the watch loop does.
func rawResource(t testing.TB, v map[string]interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal resource: %v", err)
	}
	return raw
}

// resourceMap decodes an event's resource JSON for assertions.
func resourceMap(e ResourceEvent) map[string]interface{} {
	var m map[string]interface{}
	if len(e.Resource) == 0 {
		return nil
	}
	_ = json.Unmarshal(e.Resource, &m)
	return m
}
