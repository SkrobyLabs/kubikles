package ai

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestTruncateForUI(t *testing.T) {
	// Under and at the limit are returned unchanged.
	if got := truncateForUI("hello", 10); got != "hello" {
		t.Errorf("under limit: got %q", got)
	}
	if got := truncateForUI("hello", 5); got != "hello" {
		t.Errorf("exactly at limit should not truncate: got %q", got)
	}

	// One past the limit truncates and appends the marker.
	got := truncateForUI("hello world", 5)
	if got != "hello… [truncated]" {
		t.Errorf("over limit: got %q", got)
	}
}

func TestTruncateForUI_RuneBoundary(t *testing.T) {
	// "héllo" — é is 2 bytes (0xC3 0xA9), so a byte cut at 2 would split it.
	s := "héllo"
	got := truncateForUI(s, 2)
	if !utf8.ValidString(got) {
		t.Errorf("truncated string is not valid UTF-8: %q", got)
	}
	// The cut must back up before the split rune, yielding "h" + marker.
	if got != "h… [truncated]" {
		t.Errorf("expected rune-safe cut, got %q", got)
	}
}

func TestCompactJSON(t *testing.T) {
	got := compactJSON([]byte(`{ "pod":  "nginx",
		"tail": 100 }`))
	if got != `{"pod":"nginx","tail":100}` {
		t.Errorf("expected compacted JSON, got %q", got)
	}

	// Invalid JSON falls back to the raw string.
	raw := `not json`
	if got := compactJSON([]byte(raw)); got != raw {
		t.Errorf("expected raw fallback for invalid JSON, got %q", got)
	}
}

func TestExecuteToolsAndBuildResult_NilClientEmitsErrorEvents(t *testing.T) {
	toolUses := []anthropic.ToolUseBlock{
		{ID: "t1", Name: "get_pod_logs", Input: json.RawMessage(`{"pod":"a"}`)},
		{ID: "t2", Name: "list_resources", Input: json.RawMessage(`{}`)},
	}

	var events []StreamEvent
	msg := executeToolsAndBuildResult(nil, toolUses, func(e StreamEvent) {
		events = append(events, e)
	})

	// One tool_result event per tool, all flagged as errors.
	if len(events) != 2 {
		t.Fatalf("expected 2 tool_result events, got %d", len(events))
	}
	for i, e := range events {
		if e.Type != "tool_result" {
			t.Errorf("event %d: expected type tool_result, got %q", i, e.Type)
		}
		if !e.IsError {
			t.Errorf("event %d: expected IsError=true for nil client", i)
		}
		if e.ToolID != toolUses[i].ID {
			t.Errorf("event %d: expected ToolID %q, got %q", i, toolUses[i].ID, e.ToolID)
		}
	}

	// The message still carries a tool_result block per tool for the model.
	if len(msg.Content) != 2 {
		t.Errorf("expected 2 result blocks, got %d", len(msg.Content))
	}
}

func TestExecuteToolsAndBuildResult_NilCallback(t *testing.T) {
	// A nil onEvent must not panic.
	toolUses := []anthropic.ToolUseBlock{
		{ID: "t1", Name: "get_pod_logs", Input: json.RawMessage(`{}`)},
	}
	msg := executeToolsAndBuildResult(nil, toolUses, nil)
	if len(msg.Content) != 1 {
		t.Errorf("expected 1 result block, got %d", len(msg.Content))
	}
}
