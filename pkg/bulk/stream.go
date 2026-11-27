package bulk

import (
	"errors"
	"net/http"
	"time"
)

const (
	// chunkSize is the buffering unit. Bodies reach the Wails scheme handler in
	// pieces of at most this size.
	chunkSize = 64 << 10
	// DefaultChunkTimeout is how far each flushed chunk pushes the write
	// deadline. http.Server.WriteTimeout is 60s for the whole response, which a
	// long stream would otherwise hit.
	DefaultChunkTimeout = 30 * time.Second
)

// ChunkWriter buffers into 64KB chunks, flushes each through
// http.ResponseController when the writer supports it, and extends the write
// deadline per chunk. Any write error is sticky and means the client is gone:
// the Wails asset server never cancels r.Context() (it builds requests with
// http.NewRequest) and its darwin writer returns an error once the task is
// stopped, so the error is the only cancellation signal. Producers must check
// Err (or the return value of Write/WriteLine) after every line.
type ChunkWriter struct {
	w       http.ResponseWriter
	rc      *http.ResponseController
	buf     []byte
	err     error
	timeout time.Duration
	// Bytes counts bytes handed to the underlying writer.
	Bytes int64
}

// NewChunkWriter wraps w. timeout <= 0 uses DefaultChunkTimeout.
func NewChunkWriter(w http.ResponseWriter, timeout time.Duration) *ChunkWriter {
	if timeout <= 0 {
		timeout = DefaultChunkTimeout
	}
	c := &ChunkWriter{w: w, rc: http.NewResponseController(w), buf: make([]byte, 0, chunkSize), timeout: timeout}
	c.extendDeadline()
	return c
}

func (c *ChunkWriter) extendDeadline() {
	if err := c.rc.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		c.err = err
	}
}

// Err returns the first write error, if any.
func (c *ChunkWriter) Err() error { return c.err }

// Write implements io.Writer. Large writes are split so no single write to the
// underlying writer exceeds the chunk size.
func (c *ChunkWriter) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n := 0
	for len(p) > 0 {
		room := chunkSize - len(c.buf)
		take := len(p)
		if take > room {
			take = room
		}
		c.buf = append(c.buf, p[:take]...)
		p = p[take:]
		n += take
		if len(c.buf) >= chunkSize {
			if err := c.Flush(); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// WriteLine writes b followed by a newline.
func (c *ChunkWriter) WriteLine(b []byte) error {
	if _, err := c.Write(b); err != nil {
		return err
	}
	_, err := c.Write([]byte{'\n'})
	return err
}

// Flush pushes buffered bytes to the client now.
func (c *ChunkWriter) Flush() error {
	if c.err != nil {
		return c.err
	}
	if len(c.buf) > 0 {
		n, err := c.w.Write(c.buf)
		c.Bytes += int64(n)
		c.buf = c.buf[:0]
		if err != nil {
			c.err = err
			return err
		}
	}
	// The Wails writers are not Flushers (they write through); ignore that.
	if err := c.rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		c.err = err
		return err
	}
	c.extendDeadline()
	return c.err
}
