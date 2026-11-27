package server

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// clientQueueLen bounds queued messages per client.
	clientQueueLen = 512
	// clientQueueBytes bounds queued payload bytes per client; a client that
	// falls this far behind is disconnected and re-lists on reconnect.
	clientQueueBytes = 64 << 20
	// writeWait bounds a single socket write so a stalled peer cannot pin the
	// writer goroutine forever.
	writeWait = 15 * time.Second
	// pongWait is how long the server tolerates silence from the peer.
	pongWait = 60 * time.Second
	// pingPeriod must be shorter than pongWait.
	pingPeriod = 25 * time.Second
	// maxInboundMessage: clients send nothing meaningful over the socket.
	maxInboundMessage = 1 << 20
)

type queuedMessage struct {
	msg  *websocket.PreparedMessage
	size int64
}

// wsClient owns one WebSocket connection. All writes go through a single
// writer goroutine fed by a bounded queue, so producers never block.
type wsClient struct {
	id     string
	conn   *websocket.Conn
	send   chan queuedMessage
	done   chan struct{}
	once   sync.Once
	queued atomic.Int64
	// maxQueued allows tests to shrink the byte budget.
	maxQueued int64
}

func newWSClient(id string, conn *websocket.Conn) *wsClient {
	return &wsClient{
		id:        id,
		conn:      conn,
		send:      make(chan queuedMessage, clientQueueLen),
		done:      make(chan struct{}),
		maxQueued: clientQueueBytes,
	}
}

// enqueue never blocks. It reports false and disconnects the client when the
// queue is full, so a slow consumer is cut loose instead of dropping single
// events silently (the frontend reconnects and re-lists).
func (c *wsClient) enqueue(m queuedMessage) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	// An empty queue always accepts one message: a single oversized page
	// (large CRDs) must not disconnect an otherwise healthy client, which would
	// reconnect and re-list into the same overflow.
	if prev := c.queued.Add(m.size) - m.size; prev > 0 && prev+m.size > c.maxQueued {
		c.queued.Add(-m.size)
		c.kill(nil)
		return false
	}
	select {
	case c.send <- m:
		return true
	default:
		c.queued.Add(-m.size)
		c.kill(nil)
		return false
	}
}

// kill closes the connection once. A non-nil closeMsg is sent best-effort
// first (graceful shutdown). The reader goroutine then unregisters the client.
func (c *wsClient) kill(closeMsg []byte) {
	c.once.Do(func() {
		close(c.done)
		if closeMsg != nil {
			_ = c.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))
		}
		_ = c.conn.Close()
	})
}

// writeLoop is the only goroutine that writes to the connection.
func (c *wsClient) writeLoop() {
	ping := time.NewTicker(pingPeriod)
	defer ping.Stop()
	defer c.kill(nil)
	for {
		select {
		case <-c.done:
			return
		case m := <-c.send:
			c.queued.Add(-m.size)
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WritePreparedMessage(m.msg); err != nil {
				return
			}
		case <-ping.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				return
			}
		}
	}
}

// readLoop discards inbound data, keeps the pong deadline fresh, and returns
// when the connection ends.
func (c *wsClient) readLoop() {
	c.conn.SetReadLimit(maxInboundMessage)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	}
}
