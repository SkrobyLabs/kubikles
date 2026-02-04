package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"kubikles/pkg/k8s"
	"kubikles/pkg/tools"
)

// Server implements an MCP JSON-RPC 2.0 server over stdin/stdout.
type Server struct {
	client              *k8s.Client
	allowedTools        map[string]bool // nil = all tools allowed, non-nil = only listed tools
	allowDangerousTools bool            // if false, tools marked IsDangerous will be rejected
}

// jsonRPCRequest represents an incoming JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse represents an outgoing JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP protocol types

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      serverInfo             `json:"serverInfo"`
}

type toolsListResult struct {
	Tools []tools.ToolDef `json:"tools"`
}

type toolsCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type toolsCallResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Run starts the MCP server, reading JSON-RPC from stdin and writing to stdout.
// allowedTools restricts which tools can be called; nil or empty means all tools allowed.
func Run(k8sContext string, allowedTools []string) error {
	return RunWithOptions(k8sContext, allowedTools, false)
}

// RunWithOptions starts the MCP server with additional configuration.
// allowDangerousTools allows execution of tools marked as dangerous.
func RunWithOptions(k8sContext string, allowedTools []string, allowDangerousTools bool) error {
	client, err := k8s.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create K8s client: %w", err)
	}

	if k8sContext != "" {
		if err := client.SwitchContext(k8sContext); err != nil {
			return fmt.Errorf("failed to switch to context %q: %w", k8sContext, err)
		}
	}

	var allowed map[string]bool
	if len(allowedTools) > 0 {
		allowed = make(map[string]bool, len(allowedTools))
		for _, t := range allowedTools {
			allowed[t] = true
		}
	}

	s := &Server{
		client:              client,
		allowedTools:        allowed,
		allowDangerousTools: allowDangerousTools,
	}
	return s.serve()
}

func (s *Server) serve() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.writeError(nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(req)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(req jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		s.writeResult(req.ID, initializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    map[string]interface{}{"tools": map[string]interface{}{}},
			ServerInfo:      serverInfo{Name: "kubikles", Version: "1.0.0"},
		})

	case "notifications/initialized":
		// Client acknowledgment, no response needed

	case "tools/list":
		allDefs := tools.AllToolDefs()
		if s.allowedTools != nil {
			var filtered []tools.ToolDef
			for _, t := range allDefs {
				if s.allowedTools[t.Name] {
					filtered = append(filtered, t)
				}
			}
			allDefs = filtered
		}
		s.writeResult(req.ID, toolsListResult{Tools: allDefs})

	case "tools/call":
		var params toolsCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.writeError(req.ID, -32602, "Invalid params")
			return
		}
		if s.allowedTools != nil && !s.allowedTools[params.Name] {
			s.writeResult(req.ID, toolsCallResult{
				Content: []toolContent{{Type: "text", Text: fmt.Sprintf("Tool %q is not allowed by the current configuration", params.Name)}},
				IsError: true,
			})
			return
		}
		// Validate dangerous tool access
		if err := s.validateToolCall(params.Name); err != nil {
			s.writeResult(req.ID, toolsCallResult{
				Content: []toolContent{{Type: "text", Text: err.Error()}},
				IsError: true,
			})
			return
		}
		result, isErr := tools.CallTool(s.client, params.Name, params.Arguments)
		s.writeResult(req.ID, toolsCallResult{
			Content: []toolContent{{Type: "text", Text: result}},
			IsError: isErr,
		})

	default:
		s.writeError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func (s *Server) writeResult(id json.RawMessage, result interface{}) {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", data)
}

func (s *Server) writeError(id json.RawMessage, code int, message string) {
	resp := jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
	data, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", data)
}

// validateToolCall checks if a tool can be executed based on its metadata.
// Returns an error if the tool is dangerous and dangerous tools are not allowed.
func (s *Server) validateToolCall(toolName string) error {
	meta := tools.DefaultToolRegistry.GetMeta(toolName)
	if meta.IsDangerous && !s.allowDangerousTools {
		note := meta.SafetyNote
		if note == "" {
			note = "This tool can modify cluster resources"
		}
		return fmt.Errorf("tool %q requires explicit enablement: %s", toolName, note)
	}
	return nil
}
