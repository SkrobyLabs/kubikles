package terminal

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local app
	},
}

type Service struct {
	Port int
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Start() error {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return err
	}
	s.Port = listener.Addr().(*net.TCPAddr).Port

	http.HandleFunc("/terminal", s.handleTerminal)

	go func() {
		if err := http.Serve(listener, nil); err != nil {
			log.Printf("Terminal server error: %v", err)
		}
	}()

	log.Printf("Terminal server started on port %d", s.Port)
	return nil
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

	// Get optional custom command
	customCommand := query.Get("command")

	// Construct kubectl command
	cmdArgs := []string{"exec", "-it"}
	if contextName != "" {
		cmdArgs = append(cmdArgs, "--context", contextName)
	}
	cmdArgs = append(cmdArgs, "-n", namespace, pod)
	if container != "" {
		cmdArgs = append(cmdArgs, "-c", container)
	}
	if customCommand == "nsenter" {
		// Special case for node shell - pass nsenter args directly
		cmdArgs = append(cmdArgs, "--", "nsenter", "-t", "1", "-m", "-u", "-i", "-n", "-p", "--", "/bin/sh")
	} else if customCommand != "" {
		// Use custom command wrapped in shell
		cmdArgs = append(cmdArgs, "--", "/bin/sh", "-c", customCommand)
	} else {
		// Default: try bash, fallback to sh
		cmdArgs = append(cmdArgs, "--", "/bin/sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi")
	}

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

	// Handle resizing
	// Note: In a real app, we'd handle resize messages from frontend.
	// For now, we'll set a default size or handle it if we implement the protocol.
	// xterm.js sends resize events, we can listen for them if we wrap the data.
	// For simplicity MVP, we just pipe raw data.

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

		if messageType == websocket.TextMessage {
			_, _ = io.Copy(ptmx, reader)
		} else if messageType == websocket.BinaryMessage {
			_, _ = io.Copy(ptmx, reader)
		}
	}
}
