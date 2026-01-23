//go:build !windows

package terminal

import (
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Session represents a terminal session on Unix
type Session struct {
	ID       string
	opts     SessionOptions
	pty      *os.File
	cmd      *exec.Cmd
	done     chan struct{}
	emitFunc func(TerminalEvent)
	stopOnce sync.Once
}

// newSession creates and starts a new terminal session
func newSession(id string, opts SessionOptions, emitFunc func(TerminalEvent)) (*Session, error) {
	cmdArgs := BuildKubectlArgs(opts.Namespace, opts.Pod, opts.Container, opts.Context, opts.Command)
	cmd := exec.Command("kubectl", cmdArgs...)

	// Start pty
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	s := &Session{
		ID:       id,
		opts:     opts,
		pty:      ptmx,
		cmd:      cmd,
		done:     make(chan struct{}),
		emitFunc: emitFunc,
	}

	// Start read loop
	go s.readLoop()

	return s, nil
}

// readLoop reads from PTY and emits events
func (s *Session) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in terminal readLoop session %s: %v", s.ID, r)
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, err := s.pty.Read(buf)
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
	_, err := s.pty.Write(data)
	return err
}

// Resize changes the terminal size
func (s *Session) Resize(cols, rows int) error {
	select {
	case <-s.done:
		return nil
	default:
	}
	return pty.Setsize(s.pty, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

// Close terminates the session
func (s *Session) Close() {
	s.stopOnce.Do(func() {
		close(s.done)
		if s.pty != nil {
			_ = s.pty.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	})
}
