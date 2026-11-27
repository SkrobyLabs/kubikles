package bulk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxProbeChunks   = 1000
	maxProbeDelayMs  = 5000
	maxProbeChunkLen = 1 << 20
	maxProbeObjects  = 100000
	maxProbeMB       = 64
	maxProbeDuration = 30 * time.Second
)

func intParam(r *http.Request, key string, def, min, max int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// probeStream writes N newline-terminated JSON lines, each followed by a flush
// and an optional delay, so a client can compare first-line and last-line
// arrival to tell streaming from buffering.
func (h *handler) probeStream(w http.ResponseWriter, r *http.Request) {
	chunks := intParam(r, "chunks", 5, 1, maxProbeChunks)
	delay := time.Duration(intParam(r, "delayMs", 50, 0, maxProbeDelayMs)) * time.Millisecond
	size := intParam(r, "size", 64, 0, maxProbeChunkLen)
	if time.Duration(chunks)*delay > maxProbeDuration {
		writeJSONError(w, http.StatusBadRequest, "stream would exceed probe duration limit")
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	cw := NewChunkWriter(w, h.d.ChunkTimeout)
	pad := strings.Repeat("x", size)
	for i := 0; i < chunks; i++ {
		line := fmt.Sprintf(`{"i":%d,"t":%d,"pad":"%s"}`, i, time.Now().UnixMilli(), pad)
		if cw.WriteLine([]byte(line)) != nil || cw.Flush() != nil {
			return
		}
		if delay > 0 && i < chunks-1 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
	}
}

// probeBytes writes mb megabytes of filler.
func (h *handler) probeBytes(w http.ResponseWriter, r *http.Request) {
	mb := intParam(r, "mb", 1, 1, maxProbeMB)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(mb<<20))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	cw := NewChunkWriter(w, h.d.ChunkTimeout)
	block := make([]byte, 32<<10)
	for i := range block {
		block[i] = byte('a' + i%26)
	}
	for sent := 0; sent < mb<<20; sent += len(block) {
		if _, err := cw.Write(block); err != nil {
			return
		}
	}
	_ = cw.Flush()
}

// BenchObjects returns n deterministic pod-sized objects. The same generator
// backs probe/bench and the BulkBenchIPC binding, so timings compare the
// transport and not the payload.
func BenchObjects(n int) []map[string]interface{} {
	out := make([]map[string]interface{}, n)
	for i := 0; i < n; i++ {
		out[i] = BenchObject(i)
	}
	return out
}

// BenchObject returns the i-th benchmark object.
func BenchObject(i int) map[string]interface{} {
	name := fmt.Sprintf("bench-pod-%06d", i)
	return map[string]interface{}{
		"name":      name,
		"namespace": fmt.Sprintf("ns-%d", i%20),
		"uid":       fmt.Sprintf("00000000-0000-4000-8000-%012d", i),
		"labels": map[string]string{
			"app":                       fmt.Sprintf("app-%d", i%50),
			"pod-template-hash":         fmt.Sprintf("%08x", i*2654435761%0xffffffff),
			"app.kubernetes.io/name":    "bench",
			"app.kubernetes.io/part-of": "kubikles-probe",
		},
		"status":    "Running",
		"phase":     "Running",
		"nodeName":  fmt.Sprintf("node-%d", i%12),
		"podIP":     fmt.Sprintf("10.%d.%d.%d", (i/65536)%256, (i/256)%256, i%256),
		"restarts":  i % 7,
		"createdAt": "2025-01-01T00:00:00Z",
		"containers": []map[string]interface{}{
			{"name": "main", "image": "registry.example.com/bench/main:1.2.3", "ready": true, "restartCount": i % 7,
				"resources": map[string]string{"cpu": "250m", "memory": "256Mi"}},
			{"name": "sidecar", "image": "registry.example.com/bench/sidecar:4.5.6", "ready": true, "restartCount": 0,
				"resources": map[string]string{"cpu": "50m", "memory": "64Mi"}},
		},
		"annotations": map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": strings.Repeat("a", 400),
		},
	}
}

// probeBench writes n bench objects as one JSON document ({"data":[...]}) or
// as NDJSON.
func (h *handler) probeBench(w http.ResponseWriter, r *http.Request) {
	n := intParam(r, "objects", 1000, 1, maxProbeObjects)
	mode := r.URL.Query().Get("mode")
	if mode != "ndjson" {
		mode = "json"
	}
	if mode == "ndjson" {
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	cw := NewChunkWriter(w, h.d.ChunkTimeout)
	if mode == "json" {
		_, _ = cw.Write([]byte(`{"data":[`))
	}
	for i := 0; i < n; i++ {
		line, err := json.Marshal(BenchObject(i))
		if err != nil {
			return
		}
		if mode == "ndjson" {
			if cw.WriteLine(line) != nil {
				return
			}
			continue
		}
		if i > 0 {
			if _, err := cw.Write([]byte{','}); err != nil {
				return
			}
		}
		if _, err := cw.Write(line); err != nil {
			return
		}
	}
	if mode == "json" {
		_, _ = cw.Write([]byte(`]}`))
	}
	_ = cw.Flush()
}
