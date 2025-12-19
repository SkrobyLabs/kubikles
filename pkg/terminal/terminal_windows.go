//go:build windows

package terminal

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/UserExistsError/conpty"
	"github.com/gorilla/websocket"
)

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

	// Build full command string for Windows ConPTY
	fullCommand := "kubectl " + strings.Join(cmdArgs, " ")

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
		messageType, reader, err := conn.NextReader()
		if err != nil {
			break
		}

		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			_, _ = io.Copy(cpty, reader)
		}
	}
}
