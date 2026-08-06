package realtime

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	sendBuffer   = 16
	pingInterval = 30 * time.Second
	pingTimeout  = 10 * time.Second
	writeTimeout = 5 * time.Second
)

// client wraps one WebSocket connection. Reads and writes each run in
// their own goroutine (Conn forbids concurrent Read calls, see
// coder/websocket's Conn docs), coordinated through send/done so
// either side closing cleanly stops the other — with no risk of a
// send on a closed channel, since send is never closed (only done is,
// exactly once, via sync.Once).
type client struct {
	conn *websocket.Conn
	send chan notice
	done chan struct{}
	once sync.Once
}

func newClient(conn *websocket.Conn) *client {
	return &client{conn: conn, send: make(chan notice, sendBuffer), done: make(chan struct{})}
}

func (c *client) close() {
	c.once.Do(func() { close(c.done) })
}

// notify enqueues n for delivery, dropping it if the client's buffer
// is full or it's already shutting down — safe to drop because the
// client will catch up via GET /sync/changes on its next pull.
func (c *client) notify(n notice) {
	select {
	case c.send <- n:
	default:
	}
}

func (c *client) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case n := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := wsjson.Write(wctx, c.conn, n)
			cancel()
			if err != nil {
				c.close()
				return
			}
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				_ = c.conn.Close(websocket.StatusPolicyViolation, "ping timeout")
				c.close()
				return
			}
		}
	}
}

func (c *client) readLoop(ctx context.Context) {
	defer c.close()
	for {
		if _, _, err := c.conn.Read(ctx); err != nil {
			return
		}
		// Any message after auth is ignored — this connection is
		// notify-only. Still need to keep reading so control frames
		// (ping/pong/close) are processed and a client disconnect is
		// detected promptly.
	}
}
