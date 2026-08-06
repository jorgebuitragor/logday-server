package realtime

import "sync"

// Hub tracks active WebSocket connections per user (one topic per
// user_id, not per entity type — see specs/sync-incremental).
type Hub struct {
	mu    sync.Mutex
	conns map[string]map[*client]struct{}
}

// NewHub builds an empty Hub, ready to register connections and
// receive Notify calls.
func NewHub() *Hub {
	return &Hub{conns: make(map[string]map[*client]struct{})}
}

func (h *Hub) register(userID string, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[userID] == nil {
		h.conns[userID] = make(map[*client]struct{})
	}
	h.conns[userID][c] = struct{}{}
}

func (h *Hub) unregister(userID string, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns[userID], c)
	if len(h.conns[userID]) == 0 {
		delete(h.conns, userID)
	}
}

// Notify broadcasts a lightweight change notice to every connection
// currently subscribed for userID. Safe to call when userID has no
// active connections (no-op) — domain packages call this
// unconditionally after every successful write.
func (h *Hub) Notify(userID, entityType, id string, seq int64) {
	h.mu.Lock()
	clients := make([]*client, 0, len(h.conns[userID]))
	for c := range h.conns[userID] {
		clients = append(clients, c)
	}
	h.mu.Unlock()

	n := notice{Type: entityType, ID: id, Seq: seq}
	for _, c := range clients {
		c.notify(n)
	}
}
