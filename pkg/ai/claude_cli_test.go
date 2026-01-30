package ai

import (
	"testing"
)

func TestClaudeCLIProvider_Name(t *testing.T) {
	provider := NewClaudeCLIProvider()
	name := provider.Name()

	if name != "Claude CLI" {
		t.Errorf("expected name 'Claude CLI', got %q", name)
	}
}

func TestClaudeCLIProvider_SupportsSession(t *testing.T) {
	provider := NewClaudeCLIProvider()

	if !provider.SupportsSession() {
		t.Error("expected SupportsSession to return true")
	}
}

func TestStreamParser_ParseAssistantMessage(t *testing.T) {
	parser := &streamParser{}

	// Test assistant message with text content
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`
	event := parser.parseLine(line)

	if event.Type != "text" {
		t.Errorf("expected type 'text', got %q", event.Type)
	}
	if event.Content != "Hello world" {
		t.Errorf("expected content 'Hello world', got %q", event.Content)
	}
}

func TestStreamParser_ParseResult(t *testing.T) {
	parser := &streamParser{}

	// First send an assistant message to set receivedText
	parser.parseLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}`)

	// Then parse result - should return done since we already have text
	line := `{"type":"result","result":"Hello"}`
	event := parser.parseLine(line)

	if event.Type != "done" {
		t.Errorf("expected type 'done', got %q", event.Type)
	}
	if !parser.sentDone {
		t.Error("expected sentDone to be true")
	}
}

func TestStreamParser_ParseResultFallback(t *testing.T) {
	parser := &streamParser{}

	// Parse result without prior assistant message - should use result text as fallback
	line := `{"type":"result","result":"Fallback text"}`
	event := parser.parseLine(line)

	if event.Type != "text" {
		t.Errorf("expected type 'text', got %q", event.Type)
	}
	if event.Content != "Fallback text" {
		t.Errorf("expected content 'Fallback text', got %q", event.Content)
	}
}

func TestStreamParser_ParseContentBlockDelta(t *testing.T) {
	parser := &streamParser{}

	// Test content_block_delta for incremental streaming
	line := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"incremental"}}`
	event := parser.parseLine(line)

	if event.Type != "text" {
		t.Errorf("expected type 'text', got %q", event.Type)
	}
	if event.Content != "incremental" {
		t.Errorf("expected content 'incremental', got %q", event.Content)
	}
	if !parser.receivedText {
		t.Error("expected receivedText to be true")
	}
}

func TestStreamParser_IgnoreSystemEvents(t *testing.T) {
	parser := &streamParser{}

	// System events should return empty
	line := `{"type":"system","event":"init"}`
	event := parser.parseLine(line)

	if event.Type != "" {
		t.Errorf("expected empty type for system event, got %q", event.Type)
	}
}

func TestStreamParser_InvalidJSON(t *testing.T) {
	parser := &streamParser{}

	// Invalid JSON should return empty event
	line := `not valid json`
	event := parser.parseLine(line)

	if event.Type != "" {
		t.Errorf("expected empty type for invalid JSON, got %q", event.Type)
	}
}

func TestStreamParser_AssistantWithMultipleTextBlocks(t *testing.T) {
	parser := &streamParser{}

	// Test assistant message with multiple text content blocks
	line := `{"type":"assistant","message":{"content":[{"type":"text","text":"First"},{"type":"text","text":"Second"}]}}`
	event := parser.parseLine(line)

	if event.Type != "text" {
		t.Errorf("expected type 'text', got %q", event.Type)
	}
	if event.Content != "FirstSecond" {
		t.Errorf("expected content 'FirstSecond', got %q", event.Content)
	}
}

func TestStreamParser_AssistantWithNoText(t *testing.T) {
	parser := &streamParser{}

	// Test assistant message with no text content (e.g., only tool_use)
	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"some_tool"}]}}`
	event := parser.parseLine(line)

	if event.Type != "" {
		t.Errorf("expected empty type for non-text content, got %q", event.Type)
	}
}

func TestStreamParser_ResetsForNewTurn(t *testing.T) {
	parser := &streamParser{}

	// First assistant message
	parser.parseLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"First response"}]}}`)
	if !parser.receivedText {
		t.Error("expected receivedText to be true after first message")
	}

	// New assistant message should reset receivedText
	event := parser.parseLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"Second response"}]}}`)

	if event.Type != "text" {
		t.Errorf("expected type 'text', got %q", event.Type)
	}
	if event.Content != "Second response" {
		t.Errorf("expected content 'Second response', got %q", event.Content)
	}
}

func TestStreamParser_EmptyAssistantContent(t *testing.T) {
	parser := &streamParser{}

	// Test assistant message with empty content array
	line := `{"type":"assistant","message":{"content":[]}}`
	event := parser.parseLine(line)

	if event.Type != "" {
		t.Errorf("expected empty type for empty content, got %q", event.Type)
	}
}

func TestStreamParser_EmptyTextDelta(t *testing.T) {
	parser := &streamParser{}

	// Test content_block_delta with empty text
	line := `{"type":"content_block_delta","delta":{"type":"text_delta","text":""}}`
	event := parser.parseLine(line)

	if event.Type != "" {
		t.Errorf("expected empty type for empty text delta, got %q", event.Type)
	}
}

func TestStreamParser_NonTextDelta(t *testing.T) {
	parser := &streamParser{}

	// Test content_block_delta with non-text type
	line := `{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{"}}`
	event := parser.parseLine(line)

	if event.Type != "" {
		t.Errorf("expected empty type for non-text delta, got %q", event.Type)
	}
}
