package realtime

// notice is the lightweight WebSocket payload sent when something
// changes — never the full record, see specs/sync-incremental
// ("Eventos WebSocket"). The client reacts by pulling
// GET /sync/changes from its own cursor.
type notice struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Seq  int64  `json:"seq"`
}

// authMessage is the first message a client must send after the
// WebSocket handshake completes — browsers can't set the
// Authorization header on a native WebSocket connection, so auth
// happens over the socket instead of the handshake itself.
type authMessage struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}
