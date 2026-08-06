package realtime

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/auth"
)

const authTimeout = 5 * time.Second

// Handler exposes the WebSocket upgrade endpoint. Unlike other domain
// handlers, its route does not use auth.RequireAuth middleware — the
// WebSocket handshake itself can't carry a bearer token (browsers
// don't allow setting Authorization on it), so auth happens over the
// socket via the first message instead.
type Handler struct {
	hub  *Hub
	auth *auth.Handler
}

// NewHandler builds a Handler backed by hub, authenticating
// connections against authHandler.
func NewHandler(hub *Hub, authHandler *auth.Handler) *Handler {
	return &Handler{hub: hub, auth: authHandler}
}

// Routes registers the WebSocket endpoint on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/ws", h.serveWS)
}

func (h *Handler) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// This API is meant to be consumed by clients hosted on a
		// different origin than a self-hosted instance (desktop/web/
		// mobile, per specs/arquitectura-inicial) — restricting to
		// same-origin would break that. Safe here specifically
		// because auth requires the access token in the first
		// message body, not an automatically-attached credential
		// (cookie) a hostile cross-origin page could ride on.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	userID, err := authenticate(r.Context(), h.auth, conn)
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "authentication failed")
		return
	}

	c := newClient(conn)
	h.hub.register(userID, c)
	defer h.hub.unregister(userID, c)

	go c.writeLoop(r.Context())
	c.readLoop(r.Context())
}

func authenticate(ctx context.Context, authHandler *auth.Handler, conn *websocket.Conn) (string, error) {
	actx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	var msg authMessage
	if err := wsjson.Read(actx, conn, &msg); err != nil {
		return "", err
	}
	if msg.Type != "auth" || msg.Token == "" {
		return "", errors.New("expected an auth message with a token")
	}

	return authHandler.VerifyAccessToken(msg.Token)
}
