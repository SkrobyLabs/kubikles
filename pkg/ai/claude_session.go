package ai

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Session represents a persistent CLI session for bidirectional streaming.
type Session interface {
	SendMessage(message string) error
	Close()
}

// ClaudeCLISession manages a persistent Claude CLI process for bidirectional streaming.
// Instead of spawning a new process per message, this keeps a single process running
// and communicates via stdin/stdout JSON streams.
type ClaudeCLISession struct {
	id            string
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	done          chan struct{}
	emitFunc      func(StreamEvent)
	mcpConfigPath string
	stopOnce      sync.Once
	mu            sync.Mutex
	started       bool
}

// streamInputMessage represents the JSON structure for input to Claude CLI stream-json mode.
type streamInputMessage struct {
	Type    string             `json:"type"`
	Message streamInputContent `json:"message"`
}

type streamInputContent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// newClaudeCLISession creates a new persistent Claude CLI session.
func newClaudeCLISession(id string, emitFunc func(StreamEvent)) *ClaudeCLISession {
	return &ClaudeCLISession{
		id:       id,
		done:     make(chan struct{}),
		emitFunc: emitFunc,
	}
}

// Start spawns the Claude CLI process with bidirectional streaming.
func (s *ClaudeCLISession) Start(cliPath, sessionID, systemPrompt, model, k8sContext string, allowedTools []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("session already started")
	}

	args := []string{
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--session-id", sessionID,
		"--verbose",
	}

	// Generate MCP config for K8s tools
	mcpConfigPath, err := writeMCPConfig(k8sContext, allowedTools)
	if err != nil {
		// Fall back to no-tools mode if MCP config fails
		args = append(args, "--allowedTools", "")
	} else {
		s.mcpConfigPath = mcpConfigPath
		args = append(args, "--mcp-config", mcpConfigPath)
		// Use dynamic allowedTools from request
		if len(allowedTools) > 0 {
			for _, tool := range allowedTools {
				args = append(args, "--allowedTools", tool)
			}
		} else {
			args = append(args, "--allowedTools", "")
		}
	}

	if model != "" {
		args = append(args, "--model", model)
	}

	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	s.cmd = exec.Command(cliPath, args...)
	s.cmd.Env = append(os.Environ(), "TERM=dumb")

	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		s.cleanup()
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	s.stdin = stdin

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		s.cleanup()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	s.stdout = stdout

	if err := s.cmd.Start(); err != nil {
		s.cleanup()
		return fmt.Errorf("failed to start claude CLI: %w", err)
	}

	s.started = true

	// Start read loop
	go s.readLoop()

	// Monitor process exit
	go s.waitLoop()

	return nil
}

// SendMessage sends a user message to the running Claude CLI process via stdin.
func (s *ClaudeCLISession) SendMessage(message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.done:
		return fmt.Errorf("session closed")
	default:
	}

	if s.stdin == nil {
		return fmt.Errorf("stdin not available")
	}

	// Format message as stream-json input
	input := streamInputMessage{
		Type: "user",
		Message: streamInputContent{
			Role:    "user",
			Content: message,
		},
	}

	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write JSON followed by newline
	if _, err := fmt.Fprintf(s.stdin, "%s\n", data); err != nil {
		return fmt.Errorf("failed to write to stdin: %w", err)
	}

	return nil
}

// readLoop reads from stdout and parses streaming JSON events.
func (s *ClaudeCLISession) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in Claude CLI session %s readLoop: %v", s.id, r)
		}
	}()

	scanner := bufio.NewScanner(s.stdout)
	// Allow larger lines (Claude can return large JSON)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	parser := &streamParser{}

	for scanner.Scan() {
		// Check if we should stop
		select {
		case <-s.done:
			return
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		event := parser.parseLine(line)
		if event.Type != "" {
			s.emitFunc(event)
		}

		// Reset parser state after each complete response for next message
		if event.Type == "done" {
			parser.receivedText = false
			parser.sentDone = false
		}
	}

	if err := scanner.Err(); err != nil {
		select {
		case <-s.done:
			return
		default:
			log.Printf("Claude CLI session %s scanner error: %v", s.id, err)
			s.emitFunc(StreamEvent{Type: "error", Content: fmt.Sprintf("stream read error: %v", err)})
		}
	}
}

// waitLoop monitors the CLI process and handles exit.
func (s *ClaudeCLISession) waitLoop() {
	if s.cmd == nil {
		return
	}

	err := s.cmd.Wait()

	select {
	case <-s.done:
		// Normal close, no need to emit error
		return
	default:
		// Process exited unexpectedly
		if err != nil {
			log.Printf("Claude CLI session %s process exited: %v", s.id, err)
			s.emitFunc(StreamEvent{Type: "error", Content: fmt.Sprintf("claude CLI exited: %v", err)})
		}
		s.emitFunc(StreamEvent{Type: "done"})
		s.Close()
	}
}

// Close terminates the session and cleans up resources.
func (s *ClaudeCLISession) Close() {
	s.stopOnce.Do(func() {
		close(s.done)
		s.cleanup()
	})
}

// cleanup releases resources without closing done channel.
func (s *ClaudeCLISession) cleanup() {
	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.stdout != nil {
		_ = s.stdout.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.mcpConfigPath != "" {
		_ = os.Remove(s.mcpConfigPath)
	}
}
