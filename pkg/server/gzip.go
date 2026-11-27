package server

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipMinSize: below this the CPU and header cost outweighs the saving.
const gzipMinSize = 1024

var gzipPool = sync.Pool{New: func() interface{} {
	w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
	return w
}}

var bufPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

// acceptsGzip parses Accept-Encoding, honouring q=0.
func acceptsGzip(h string) bool {
	for _, part := range strings.Split(h, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		name := strings.ToLower(strings.TrimSpace(fields[0]))
		if name != "gzip" && name != "*" {
			continue
		}
		for _, p := range fields[1:] {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "q=") {
				if q, err := strconv.ParseFloat(p[2:], 64); err == nil && q == 0 {
					return false
				}
			}
		}
		return true
	}
	return false
}

// writeBody sends body, gzipped when the client allows it and it is large.
func writeBody(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	w.Header().Add("Vary", "Accept-Encoding")
	if len(body) >= gzipMinSize && acceptsGzip(r.Header.Get("Accept-Encoding")) {
		buf := bufPool.Get().(*bytes.Buffer)
		buf.Reset()
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(buf)
		_, werr := gz.Write(body)
		cerr := gz.Close()
		gzipPool.Put(gz)
		if werr == nil && cerr == nil {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
			w.WriteHeader(status)
			_, _ = w.Write(buf.Bytes())
			// Drop very large buffers rather than pinning them in the pool.
			if buf.Cap() <= 8<<20 {
				bufPool.Put(buf)
			}
			return
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
