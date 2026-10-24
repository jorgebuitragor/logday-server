package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jorgebuitragor/logday-server/internal/db"
	"github.com/jorgebuitragor/logday-server/internal/settings"
)

// setupPolicyTestServer mirrors internal/realtime's setupServer/login
// pattern — a real HTTP server with just the auth handler mounted, an
// admin bootstrapped from ADMIN_EMAIL/ADMIN_PASSWORD. Returns the raw
// *sql.DB too, so tests can reach into internal/settings directly to
// simulate an admin bumping the policy version out from under a
// client mid-flow.
func setupPolicyTestServer(t *testing.T) (*httptest.Server, string, *sql.DB) {
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

	authStore := NewStore(database)
	if err := Bootstrap(context.Background(), authStore); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	authHandler := NewHandler(authStore, []byte("test-secret"))

	r := chi.NewRouter()
	authHandler.Routes(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	access, _ := loginForPolicyTest(t, srv, "admin@example.com", "test-password-123")
	return srv, access, database
}

func loginForPolicyTest(t *testing.T, srv *httptest.Server, email, password string) (accessToken string, tok tokenResponse) {
	t.Helper()
	body := strings.NewReader(`{"email":"` + email + `","password":"` + password + `","device_name":"test"}`)
	resp, err := http.Post(srv.URL+"/auth/login", "application/json", body)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: unexpected status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		t.Fatalf("decoding login response: %v", err)
	}
	return tok.AccessToken, tok
}

func authedRequest(t *testing.T, method, url, token, body string) *http.Response {
	t.Helper()
	var reqBody *strings.Reader
	if body == "" {
		reqBody = strings.NewReader("")
	} else {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func TestPolicyIsPublicAndLoginReportsAcceptance(t *testing.T) {
	srv, access, _ := setupPolicyTestServer(t)

	// GET /policy no requiere Authorization.
	resp, err := http.Get(srv.URL + "/policy")
	if err != nil {
		t.Fatalf("GET /policy: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /policy: expected 200, got %d", resp.StatusCode)
	}
	var pol policyResponse
	if err := json.NewDecoder(resp.Body).Decode(&pol); err != nil {
		t.Fatalf("decoding policy response: %v", err)
	}
	if pol.Version != 1 || !strings.Contains(pol.Text, "PLANTILLA") {
		t.Fatalf("expected the seeded template at version 1, got %+v", pol)
	}

	// El login recién hecho todavía no aceptó nada.
	_, tok := loginForPolicyTest(t, srv, "admin@example.com", "test-password-123")
	if tok.PolicyVersion != 1 || tok.PolicyAcceptedVersion != nil {
		t.Fatalf("expected policy_version=1 and no accepted version yet, got %+v", tok)
	}

	// Aceptar, y el próximo login ya lo refleja.
	acceptResp := authedRequest(t, http.MethodPost, srv.URL+"/policy/accept", access, `{"version":1}`)
	_ = acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /policy/accept: expected 204, got %d", acceptResp.StatusCode)
	}

	_, tok2 := loginForPolicyTest(t, srv, "admin@example.com", "test-password-123")
	if tok2.PolicyAcceptedVersion == nil || *tok2.PolicyAcceptedVersion != 1 {
		t.Fatalf("expected policy_accepted_version=1 after accepting, got %v", tok2.PolicyAcceptedVersion)
	}
}

func TestAcceptPolicyStaleVersionReturns409(t *testing.T) {
	srv, access, database := setupPolicyTestServer(t)
	ctx := context.Background()

	// El admin sube la política a la versión 2 entre que el cliente la
	// leyó y que manda la aceptación de la 1 — debe rechazarse, no
	// marcarse como aceptada una versión vieja.
	cfg, err := settings.Get(ctx, database)
	if err != nil {
		t.Fatalf("settings.Get: %v", err)
	}
	cfg.PrivacyPolicyVersion = 2
	if err := settings.Update(ctx, database, *cfg); err != nil {
		t.Fatalf("settings.Update: %v", err)
	}

	resp := authedRequest(t, http.MethodPost, srv.URL+"/policy/accept", access, `{"version":1}`)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for a stale policy version, got %d", resp.StatusCode)
	}

	// La versión vigente (2) sí se puede aceptar.
	okResp := authedRequest(t, http.MethodPost, srv.URL+"/policy/accept", access, `{"version":2}`)
	_ = okResp.Body.Close()
	if okResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 accepting the current version, got %d", okResp.StatusCode)
	}
}

func TestAcceptSensitiveDataEndpoint(t *testing.T) {
	srv, access, _ := setupPolicyTestServer(t)

	resp := authedRequest(t, http.MethodPost, srv.URL+"/policy/accept-sensitive", access, "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /policy/accept-sensitive: expected 204, got %d", resp.StatusCode)
	}
}

func TestExportAndDeleteAccountEndpoints(t *testing.T) {
	srv, access, _ := setupPolicyTestServer(t)

	exportResp := authedRequest(t, http.MethodGet, srv.URL+"/account/export", access, "")
	defer func() { _ = exportResp.Body.Close() }()
	if exportResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /account/export: expected 200, got %d", exportResp.StatusCode)
	}
	var data map[string]any
	if err := json.NewDecoder(exportResp.Body).Decode(&data); err != nil {
		t.Fatalf("decoding export: %v", err)
	}
	if _, ok := data["tasks"]; !ok {
		t.Fatalf("expected a \"tasks\" key in the export, got %v", data)
	}
	if _, ok := data["devices"]; !ok {
		t.Fatalf("expected a \"devices\" key in the export, got %v", data)
	}

	// Contraseña incorrecta: rechazado, la cuenta sigue viva.
	badDelete := authedRequest(t, http.MethodDelete, srv.URL+"/account", access, `{"password":"wrong-password"}`)
	_ = badDelete.Body.Close()
	if badDelete.StatusCode != http.StatusUnauthorized {
		t.Fatalf("DELETE /account with wrong password: expected 401, got %d", badDelete.StatusCode)
	}

	// Segundo admin, para que borrar al primero no viole la regla de
	// "no te quedes sin admin" (ver TestDeleteAccountRejectsLastAdmin
	// para ese caso).
	createResp := authedRequest(t, http.MethodPost, srv.URL+"/admin/users", access,
		`{"email":"admin2@example.com","password":"test-password-123","is_admin":true}`)
	_ = createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("creating second admin: expected 201, got %d", createResp.StatusCode)
	}

	// Contraseña correcta, ya no es el único admin: la cuenta se borra.
	okDelete := authedRequest(t, http.MethodDelete, srv.URL+"/account", access, `{"password":"test-password-123"}`)
	_ = okDelete.Body.Close()
	if okDelete.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /account with correct password: expected 204, got %d", okDelete.StatusCode)
	}

	// El access token ya emitido sigue siendo un JWT válido hasta que
	// expire por su cuenta — mismo diseño stateless que el resto del
	// sistema, la revocación real pasa por el refresh token, no por
	// invalidar el access token en el momento. Lo que sí debe fallar
	// de inmediato es un login nuevo con esas credenciales: la cuenta
	// ya no existe.
	loginResp, err := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"admin@example.com","password":"test-password-123","device_name":"test"}`))
	if err != nil {
		t.Fatalf("login after deletion: %v", err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected login to fail after account deletion, got %d", loginResp.StatusCode)
	}
}

// TestDeleteAccountRejectsLastAdmin covers the bug this repo's
// deleteUserAccount used to have: a sole admin self-deleting via
// DELETE /account (no other guard rail — the request carries their
// own correct password) would silently reopen unauthenticated
// /setup, or leave the instance with zero admins. withLastAdminGuard
// closes that the same way it already did for the admin-panel's
// promote/demote/soft-delete paths.
func TestDeleteAccountRejectsLastAdmin(t *testing.T) {
	srv, access, _ := setupPolicyTestServer(t)

	resp := authedRequest(t, http.MethodDelete, srv.URL+"/account", access, `{"password":"test-password-123"}`)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("DELETE /account as the only admin: expected 409, got %d", resp.StatusCode)
	}

	// La cuenta sigue viva: el login original todavía funciona.
	loginResp, err := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"admin@example.com","password":"test-password-123","device_name":"test"}`))
	if err != nil {
		t.Fatalf("login after rejected deletion: %v", err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected login to still work after rejected deletion, got %d", loginResp.StatusCode)
	}
}
