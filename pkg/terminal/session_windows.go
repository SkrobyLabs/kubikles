//go:build windows

package terminal

import (
	"io"
	"log"
	"strings"
	"sync"

	"github.com/UserExistsError/conpty"
)

// Session represents a terminal session on Windows
type Session struct {
	ID       string
	opts     SessionOptions
	cpty     *conpty.ConPty
	done     chan struct{}
	emitFunc func(TerminalEvent)
	stopOnce sync.Once
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

// newSession creates and starts a new terminal session
func newSession(id string, opts SessionOptions, emitFunc func(TerminalEvent)) (*Session, error) {
	cmdArgs := BuildKubectlArgs(opts.Namespace, opts.Pod, opts.Container, opts.Context, opts.Command)

	// Build full command string for Windows ConPTY, quoting args as needed
	quotedArgs := make([]string, len(cmdArgs))
	for i, arg := range cmdArgs {
		quotedArgs[i] = quoteArg(arg)
	}
	fullCommand := "kubectl " + strings.Join(quotedArgs, " ")

	// Start ConPTY with default size (80x25)
	cpty, err := conpty.Start(fullCommand, conpty.ConPtyDimensions(80, 25))
	if err != nil {
		return nil, err
	}

	s := &Session{
		ID:       id,
		opts:     opts,
		cpty:     cpty,
		done:     make(chan struct{}),
		emitFunc: emitFunc,
	}

	// Start read loop
	go s.readLoop()

	return s, nil
}

// readLoop reads from ConPTY and emits events
func (s *Session) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in terminal readLoop session %s: %v", s.ID, r)
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, err := s.cpty.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Terminal session %s read error: %v", s.ID, err)
			}
			break
		}

		// Check if we should stop before emitting
		select {
		case <-s.done:
			return
		default:
		}

		// Emit output event - convert bytes to string for JSON serialization
		s.emitFunc(TerminalEvent{
			SessionID: s.ID,
			Data:      string(buf[:n]),
		})
	}

	// Send done event (only if main loop hasn't closed us)
	select {
	case <-s.done:
		return
	default:
		s.emitFunc(TerminalEvent{
			SessionID: s.ID,
			Done:      true,
		})
	}
}

// Write sends input to the terminal
func (s *Session) Write(data []byte) error {
	select {
	case <-s.done:
		return nil
	default:
	}
	_, err := s.cpty.Write(data)
	return err
}

// Resize changes the terminal size
func (s *Session) Resize(cols, rows int) error {
	select {
	case <-s.done:
		return nil
	default:
	}
	return s.cpty.Resize(cols, rows)
}

// Close terminates the session
func (s *Session) Close() {
	s.stopOnce.Do(func() {
		close(s.done)
		if s.cpty != nil {
			s.cpty.Close()
		}
	})
}
