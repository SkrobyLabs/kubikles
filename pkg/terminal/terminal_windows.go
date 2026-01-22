//go:build windows

package terminal

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/UserExistsError/conpty"
	"github.com/gorilla/websocket"
)

// resizeMessage represents a terminal resize request from the frontend
type resizeMessage struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// quoteArg quotes an argument for Windows command line if needed
func quoteArg(arg string) string {
	// If arg contains spaces, quotes, or special shell characters, wrap in quotes
	if strings.ContainsAny(arg, " \t\";&|<>()") {
		// Escape existing quotes and wrap in quotes
		escaped := strings.ReplaceAll(arg, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return arg
}

func (s *Service) handleTerminal(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	query := r.URL.Query()
	namespace := query.Get("namespace")
	pod := query.Get("pod")
	container := query.Get("container")
	contextName := query.Get("context")

	if namespace == "" || pod == "" {
		conn.WriteMessage(websocket.TextMessage, []byte("Missing namespace or pod"))
		return
	}

	customCommand := query.Get("command")
	cmdArgs := buildKubectlArgs(namespace, pod, container, contextName, customCommand)

	// Build full command string for Windows ConPTY, quoting args as needed
	quotedArgs := make([]string, len(cmdArgs))
	for i, arg := range cmdArgs {
		quotedArgs[i] = quoteArg(arg)
	}
	fullCommand := "kubectl " + strings.Join(quotedArgs, " ")

	// Start ConPTY with default size (80x25)
	cpty, err := conpty.Start(fullCommand, conpty.ConPtyDimensions(80, 25))
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Failed to start pty: %v", err)))
		return
	}
	defer cpty.Close()

	// Pipe ConPTY output to websocket
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := cpty.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Read error: %v", err)
				}
				break
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				break
			}
		}
		// Send a clean exit message when pty closes
		conn.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[33mProcess exited. Terminal disconnected.\x1b[0m\r\n"))
		conn.Close()
	}()

	// Pipe websocket to ConPTY
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			// Check if this is a resize message (JSON starting with {"type":"resize"
			if len(data) > 0 && data[0] == '{' {
				var msg resizeMessage
				if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
					if err := cpty.Resize(msg.Cols, msg.Rows); err != nil {
						log.Printf("ConPTY resize failed: %v", err)
					}
					continue
				}
			}
			// Regular input - send to PTY
			_, _ = cpty.Write(data)
		}
	}
}
