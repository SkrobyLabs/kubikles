package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		os.Exit(1)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		os.Exit(1)
	}
	defer listener.Close()
	var count atomic.Uint64
	write := func(path string, value any) error {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return marshalErr
		}
		temporary, createErr := os.CreateTemp(filepath.Dir(path), ".accelerator-tripwire-*")
		if createErr != nil {
			return createErr
		}
		temporaryPath := temporary.Name()
		defer os.Remove(temporaryPath)
		if temporary.Chmod(0o600) != nil {
			return os.ErrInvalid
		}
		if _, writeErr := temporary.Write(append(encoded, '\n')); writeErr != nil {
			return os.ErrInvalid
		}
		if temporary.Sync() != nil || temporary.Close() != nil {
			return os.ErrInvalid
		}
		return os.Rename(temporaryPath, path)
	}
	if write(os.Args[1], listener.Addr().String()) != nil || write(os.Args[2], uint64(0)) != nil {
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		listener.Close()
	}()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil {
				return
			}
			os.Exit(1)
		}
		_ = connection.SetDeadline(time.Now().Add(time.Second))
		_ = connection.Close()
		observed := count.Add(1)
		if write(os.Args[2], observed) != nil {
			os.Exit(1)
		}
	}
}
