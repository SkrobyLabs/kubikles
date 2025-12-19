//go:build !windows

package terminal

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"

	"github.com/creack/pty"
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

	cmd := exec.Command("kubectl", cmdArgs...)

	// Start pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Failed to start pty: %v", err)))
		return
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
	}()

	// Pipe pty to websocket
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
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

	// Pipe websocket to pty
	for {
		messageType, reader, err := conn.NextReader()
		if err != nil {
			break
		}

		if messageType == websocket.TextMessage || messageType == websocket.BinaryMessage {
			_, _ = io.Copy(ptmx, reader)
		}
	}
}
