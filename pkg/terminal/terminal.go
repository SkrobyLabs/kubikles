package terminal

import (
	"log"
	"net"
	"net/http"

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

// buildKubectlArgs constructs the kubectl exec command arguments
func buildKubectlArgs(namespace, pod, container, contextName, customCommand string) []string {
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
	return cmdArgs
}
