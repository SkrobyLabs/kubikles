// accelerator00-echo is a disposable Kind-only bounded echo fixture.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net/http"
	"os"
	"time"
)

const limit = 1024 * 1024

func main() {
	mode := flag.String("mode", "serve", "serve or probe")
	address := flag.String("address", "127.0.0.1:8080", "listen address")
	marker := flag.String("marker", "target", "fixed fixture marker")
	probeURL := flag.String("url", "", "probe URL")
	flag.Parse()
	if *mode == "probe" {
		if *probeURL == "" {
			os.Exit(2)
		}
		client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
		response, err := client.Get(*probeURL)
		if err != nil {
			os.Exit(3)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(io.LimitReader(response.Body, 128))
		if err != nil || response.StatusCode != http.StatusOK || string(body) != *marker {
			os.Exit(4)
		}
		return
	}
	if *mode != "serve" {
		os.Exit(2)
	}
	mux := http.NewServeMux()
	server := &http.Server{Addr: *address, Handler: mux}
	mux.HandleFunc("/id", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, *marker) })
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, limit+1)
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) > limit {
			http.Error(w, "limit", http.StatusRequestEntityTooLarge)
			return
		}
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/terminate", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		if flush, ok := w.(http.Flusher); ok {
			flush.Flush()
		}
		go func() {
			shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		}()
	})
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		os.Exit(1)
	}
}
