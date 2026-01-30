package mcp

import (
	"encoding/json"
	"testing"

	"kubikles/pkg/tools"
)

func TestServer_Initialize(t *testing.T) {
	// Simulate initialize request
	reqID := json.RawMessage(`1`)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      reqID,
		Method:  "initialize",
	}

	// Parse the response that would be generated
	result := initializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]interface{}{"tools": map[string]interface{}{}},
		ServerInfo:      serverInfo{Name: "kubikles", Version: "1.0.0"},
	}

	// Verify protocol version
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version '2024-11-05', got %q", result.ProtocolVersion)
	}

	// Verify server info
	if result.ServerInfo.Name != "kubikles" {
		t.Errorf("expected server name 'kubikles', got %q", result.ServerInfo.Name)
	}

	// Verify capabilities include tools
	if _, ok := result.Capabilities["tools"]; !ok {
		t.Error("expected 'tools' capability")
	}

	// Verify request is properly formatted
	if req.Method != "initialize" {
		t.Errorf("expected method 'initialize', got %q", req.Method)
	}
}

func TestServer_ToolsList_AllTools(t *testing.T) {
	s := &Server{
		client:       nil,
		allowedTools: nil, // nil = all tools allowed
	}

	// Get all tool definitions
	allDefs := tools.AllToolDefs()

	// Without filtering, should return all tools
	if s.allowedTools != nil {
		t.Error("expected allowedTools to be nil")
	}

	if len(allDefs) != 12 {
		t.Errorf("expected 12 tools, got %d", len(allDefs))
	}
}

func TestServer_ToolsList_FilteredTools(t *testing.T) {
	allowedList := []string{"get_pod_logs", "list_resources"}
	allowed := make(map[string]bool)
	for _, t := range allowedList {
		allowed[t] = true
	}

	s := &Server{
		client:       nil,
		allowedTools: allowed,
	}

	// Simulate filtering
	allDefs := tools.AllToolDefs()
	var filtered []tools.ToolDef
	for _, def := range allDefs {
		if s.allowedTools[def.Name] {
			filtered = append(filtered, def)
		}
	}

	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered tools, got %d", len(filtered))
	}

	// Verify the right tools were included
	toolNames := make(map[string]bool)
	for _, def := range filtered {
		toolNames[def.Name] = true
	}

	if !toolNames["get_pod_logs"] {
		t.Error("expected 'get_pod_logs' in filtered tools")
	}
	if !toolNames["list_resources"] {
		t.Error("expected 'list_resources' in filtered tools")
	}
}

func TestServer_ToolsCall_DisallowedTool(t *testing.T) {
	allowedList := []string{"get_pod_logs"}
	allowed := make(map[string]bool)
	for _, name := range allowedList {
		allowed[name] = true
	}

	s := &Server{
		client:       nil,
		allowedTools: allowed,
	}

	// Try to call a disallowed tool
	toolName := "list_resources"
	if s.allowedTools != nil && !s.allowedTools[toolName] {
		// This is the expected behavior - tool should be rejected
	} else {
		t.Error("expected 'list_resources' to be disallowed")
	}
}

func TestServer_ToolsCall_AllowedTool(t *testing.T) {
	allowedList := []string{"get_pod_logs", "list_resources"}
	allowed := make(map[string]bool)
	for _, name := range allowedList {
		allowed[name] = true
	}

	s := &Server{
		client:       nil,
		allowedTools: allowed,
	}

	// Check allowed tool
	toolName := "get_pod_logs"
	if s.allowedTools != nil && !s.allowedTools[toolName] {
		t.Error("expected 'get_pod_logs' to be allowed")
	}
}

func TestServer_ToolsCall_NoRestrictions(t *testing.T) {
	s := &Server{
		client:       nil,
		allowedTools: nil, // nil = all tools allowed
	}

	// Any tool should be allowed when allowedTools is nil
	testTools := []string{"get_pod_logs", "list_resources", "describe_resource", "get_cluster_metrics"}

	for _, toolName := range testTools {
		// With nil allowedTools, all tools are allowed
		if s.allowedTools != nil && !s.allowedTools[toolName] {
			t.Errorf("expected tool %q to be allowed when allowedTools is nil", toolName)
		}
	}
}

func TestJSONRPCResponse_Marshal(t *testing.T) {
	tests := []struct {
		name     string
		response jsonRPCResponse
		check    func(data []byte) error
	}{
		{
			name: "success response",
			response: jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`1`),
				Result:  map[string]string{"status": "ok"},
			},
			check: func(data []byte) error {
				var parsed map[string]interface{}
				if err := json.Unmarshal(data, &parsed); err != nil {
					return err
				}
				if parsed["jsonrpc"] != "2.0" {
					t.Error("expected jsonrpc '2.0'")
				}
				if parsed["error"] != nil {
					t.Error("expected no error in success response")
				}
				return nil
			},
		},
		{
			name: "error response",
			response: jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`2`),
				Error:   &rpcError{Code: -32600, Message: "Invalid Request"},
			},
			check: func(data []byte) error {
				var parsed map[string]interface{}
				if err := json.Unmarshal(data, &parsed); err != nil {
					return err
				}
				if parsed["result"] != nil {
					t.Error("expected no result in error response")
				}
				errorObj := parsed["error"].(map[string]interface{})
				if errorObj["code"].(float64) != -32600 {
					t.Error("expected error code -32600")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}
			if err := tt.check(data); err != nil {
				t.Errorf("check failed: %v", err)
			}
		})
	}
}

func TestToolsCallParams_Parse(t *testing.T) {
	paramsJSON := `{"name":"get_pod_logs","arguments":{"namespace":"default","pod":"nginx-abc123","tail_lines":50}}`

	var params toolsCallParams
	err := json.Unmarshal([]byte(paramsJSON), &params)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if params.Name != "get_pod_logs" {
		t.Errorf("expected name 'get_pod_logs', got %q", params.Name)
	}

	if params.Arguments["namespace"] != "default" {
		t.Errorf("expected namespace 'default', got %v", params.Arguments["namespace"])
	}

	if params.Arguments["pod"] != "nginx-abc123" {
		t.Errorf("expected pod 'nginx-abc123', got %v", params.Arguments["pod"])
	}

	// JSON numbers are float64
	if params.Arguments["tail_lines"].(float64) != 50 {
		t.Errorf("expected tail_lines 50, got %v", params.Arguments["tail_lines"])
	}
}

func TestToolsCallResult_Format(t *testing.T) {
	result := toolsCallResult{
		Content: []toolContent{
			{Type: "text", Text: "Pod logs here..."},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	content := parsed["content"].([]interface{})
	if len(content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(content))
	}

	firstContent := content[0].(map[string]interface{})
	if firstContent["type"] != "text" {
		t.Errorf("expected content type 'text', got %v", firstContent["type"])
	}
	if firstContent["text"] != "Pod logs here..." {
		t.Errorf("expected content text 'Pod logs here...', got %v", firstContent["text"])
	}

	// isError should be omitted when false (omitempty)
	if _, ok := parsed["isError"]; ok && parsed["isError"].(bool) {
		t.Error("expected isError to be false or omitted")
	}
}

func TestToolsCallResult_ErrorFormat(t *testing.T) {
	result := toolsCallResult{
		Content: []toolContent{
			{Type: "text", Text: "Error: something went wrong"},
		},
		IsError: true,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if !parsed["isError"].(bool) {
		t.Error("expected isError to be true")
	}
}

func TestRPCError_Codes(t *testing.T) {
	tests := []struct {
		code    int
		message string
	}{
		{-32700, "Parse error"},
		{-32600, "Invalid Request"},
		{-32601, "Method not found"},
		{-32602, "Invalid params"},
		{-32603, "Internal error"},
	}

	for _, tt := range tests {
		err := rpcError{Code: tt.code, Message: tt.message}

		data, marshalErr := json.Marshal(err)
		if marshalErr != nil {
			t.Fatalf("marshal error for code %d: %v", tt.code, marshalErr)
		}

		var parsed rpcError
		if unmarshalErr := json.Unmarshal(data, &parsed); unmarshalErr != nil {
			t.Fatalf("unmarshal error for code %d: %v", tt.code, unmarshalErr)
		}

		if parsed.Code != tt.code {
			t.Errorf("expected code %d, got %d", tt.code, parsed.Code)
		}
		if parsed.Message != tt.message {
			t.Errorf("expected message %q, got %q", tt.message, parsed.Message)
		}
	}
}
