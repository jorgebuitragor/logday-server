package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/auth"
	"github.com/jorgebuitragor/logday-server/internal/db"
)

// hasClient is a test-only helper reaching into Hub's unexported
// state to synchronize with the server's async registration —
// avoids a sleep-and-hope race between the client's auth message and
// the test calling Notify.
func (h *Hub) hasClient(userID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.conns[userID]) > 0
}

func setupServer(t *testing.T) (*httptest.Server, *Hub) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}

	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	t.Setenv("ADMIN_PASSWORD", "test-password-123")

	authStore := auth.NewStore(database)
	if err := auth.Bootstrap(context.Background(), authStore); err != nil {
		t.Fatalf("auth.Bootstrap: %v", err)
	}
	authHandler := auth.NewHandler(authStore, []byte("test-secret"))

	hub := NewHub()
	realtimeHandler := NewHandler(hub, authHandler)

	r := chi.NewRouter()
	authHandler.Routes(r)
	realtimeHandler.Routes(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, hub
}

func login(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	body := strings.NewReader(`{"email":"admin@example.com","password":"test-password-123","device_name":"test"}`)
	resp, err := http.Post(srv.URL+"/auth/login", "application/json", body)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: unexpected status %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding login response: %v", err)
	}
	return out.AccessToken
}

// userIDFromToken reads the JWT payload's "sub" claim directly
// (unverified — this is a test, and the token was just minted by our
// own server) rather than needing a new export from internal/auth
// just to look up the id.
func userIDFromToken(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed token: %q", token)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decoding token payload: %v", err)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshaling claims: %v", err)
	}
	return claims.Sub
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

func TestServeWSDeliversNotify(t *testing.T) {
	srv, hub := setupServer(t)
	token := login(t, srv)
	userID := userIDFromToken(t, token)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(srv), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	if err := wsjson.Write(ctx, conn, authMessage{Type: "auth", Token: token}); err != nil {
		t.Fatalf("writing auth message: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !hub.hasClient(userID) {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for websocket registration")
		}
		time.Sleep(10 * time.Millisecond)
	}

	hub.Notify(userID, "task", "task-1", 42)

	var got notice
	if err := wsjson.Read(ctx, conn, &got); err != nil {
		t.Fatalf("reading notice: %v", err)
	}
	if got.Type != "task" || got.ID != "task-1" || got.Seq != 42 {
		t.Fatalf("unexpected notice: %+v", got)
	}
}

func TestServeWSRejectsMissingAuth(t *testing.T) {
	srv, _ := setupServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(srv), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	if err := wsjson.Write(ctx, conn, map[string]string{"type": "not-auth"}); err != nil {
		t.Fatalf("writing message: %v", err)
	}

	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("expected connection to be closed after an invalid auth message")
	}
}

func TestServeWSRejectsInvalidToken(t *testing.T) {
	srv, _ := setupServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL(srv), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	if err := wsjson.Write(ctx, conn, authMessage{Type: "auth", Token: "not-a-real-token"}); err != nil {
		t.Fatalf("writing auth message: %v", err)
	}

	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("expected connection to be closed after an invalid token")
	}
}
