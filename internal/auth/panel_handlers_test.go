package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"github.com/jorgebuitragor/logday-server/internal/db"
	"github.com/jorgebuitragor/logday-server/internal/security"
)

func setupPanelServer(t *testing.T) (*httptest.Server, *Handler, *store) {
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

	authStore := NewStore(database)
	authHandler := NewHandler(authStore, []byte("test-secret"))

	r := chi.NewRouter()
	authHandler.Routes(r)
	authHandler.PanelRoutes(r)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, authHandler, authStore
}

// newPanelClient returns an http.Client with a cookie jar (so session
// and CSRF cookies persist across requests, like a browser) that does
// NOT auto-follow redirects — tests assert on the 302 and its Location
// header directly instead of chasing it.
func newPanelClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

var csrfInputRe = regexp.MustCompile(`name="csrf_token" value="([^"]*)"`)

// getAndScrapeCSRF GETs path and returns both the response body (for
// further assertions) and the csrf_token value rendered into its form —
// the same double-submit token the client's jar just received as a
// cookie, so a subsequent POST from the same client authenticates.
func getAndScrapeCSRF(t *testing.T, client *http.Client, srv *httptest.Server, path string) (body string, resp *http.Response) {
	t.Helper()
	resp, err := client.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body of %s: %v", path, err)
	}
	return string(b), resp
}

func scrapeCSRF(t *testing.T, body string) string {
	t.Helper()
	m := csrfInputRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no csrf_token field found in body:\n%s", body)
	}
	return m[1]
}

func postForm(t *testing.T, client *http.Client, srv *httptest.Server, path string, values url.Values) *http.Response {
	t.Helper()
	resp, err := client.PostForm(srv.URL+path, values)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func TestSetupFlowCreatesAdminOnceThenLocksItself(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	client := newPanelClient(t)

	body, resp := getAndScrapeCSRF(t, client, srv, "/setup")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /setup: expected 200, got %d", resp.StatusCode)
	}
	csrf := scrapeCSRF(t, body)

	resp = postForm(t, client, srv, "/setup", url.Values{
		"csrf_token":       {csrf},
		"email":            {"admin@example.com"},
		"password":         {"correct-horse-battery"},
		"password_confirm": {"correct-horse-battery"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/admin/panel" {
		t.Fatalf("POST /setup: expected 302 to /admin/panel, got %d Location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp.Header.Get("Set-Cookie") == "" {
		t.Fatalf("expected POST /setup to set a session cookie")
	}

	count, err := authStore.countUsers(context.Background())
	if err != nil {
		t.Fatalf("countUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 user after setup, got %d", count)
	}

	// The setup screen must lock itself once an admin exists — a fresh
	// client (no session) hitting /setup again should be bounced to
	// login, not shown the form again.
	fresh := newPanelClient(t)
	resp2, err := fresh.Get(srv.URL + "/setup")
	if err != nil {
		t.Fatalf("second GET /setup: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusFound || resp2.Header.Get("Location") != "/admin/panel/login" {
		t.Fatalf("expected /setup to redirect to login once initialized, got %d Location=%q",
			resp2.StatusCode, resp2.Header.Get("Location"))
	}
}

func mustCreatePanelAdmin(t *testing.T, s *store, email, password string) *user {
	t.Helper()
	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	u, err := s.createUser(context.Background(), email, hash, true)
	if err != nil {
		t.Fatalf("createUser: %v", err)
	}
	return u
}

func loginToPanel(t *testing.T, client *http.Client, srv *httptest.Server, email, password string) {
	t.Helper()
	body, _ := getAndScrapeCSRF(t, client, srv, "/admin/panel/login")
	csrf := scrapeCSRF(t, body)

	resp := postForm(t, client, srv, "/admin/panel/login", url.Values{
		"csrf_token": {csrf},
		"email":      {email},
		"password":   {password},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/admin/panel" {
		t.Fatalf("panel login: expected 302 to /admin/panel, got %d Location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestPanelLoginAuthenticatesAndRejectsNonAdmin(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")
	memberHash, err := security.HashPassword("member-pass-123")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := authStore.createUser(context.Background(), "member@example.com", memberHash, false); err != nil {
		t.Fatalf("createUser member: %v", err)
	}

	client := newPanelClient(t)
	loginToPanel(t, client, srv, "admin@example.com", "correct-horse-battery")

	resp, err := client.Get(srv.URL + "/admin/panel")
	if err != nil {
		t.Fatalf("GET /admin/panel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/panel: expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(b), "admin@example.com") {
		t.Fatalf("expected the user list to contain the logged-in admin's email")
	}

	// A non-admin must not be able to log into the panel at all, even
	// with correct credentials.
	nonAdminClient := newPanelClient(t)
	body, _ := getAndScrapeCSRF(t, nonAdminClient, srv, "/admin/panel/login")
	csrf := scrapeCSRF(t, body)
	resp2 := postForm(t, nonAdminClient, srv, "/admin/panel/login", url.Values{
		"csrf_token": {csrf}, "email": {"member@example.com"}, "password": {"member-pass-123"},
	})
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected a non-admin panel login to be rejected (401), got %d", resp2.StatusCode)
	}
}

func TestPanelFormRejectsMissingCSRF(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")

	client := newPanelClient(t)
	// Visiting the login page first populates the CSRF cookie in the
	// jar, but we deliberately omit it from the form body below.
	getAndScrapeCSRF(t, client, srv, "/admin/panel/login")

	resp := postForm(t, client, srv, "/admin/panel/login", url.Values{
		"email": {"admin@example.com"}, "password": {"correct-horse-battery"},
	})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for a form POST missing csrf_token, got %d", resp.StatusCode)
	}
}

func TestPanelSessionRedirectsOnTamperedOrExpiredCookie(t *testing.T) {
	srv, h, authStore := setupPanelServer(t)
	admin := mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")

	// Tampered: well-formed cookie value, garbage token.
	tampered := newPanelClient(t)
	setRawSessionCookie(t, tampered, srv, "not-a-real-jwt")
	assertRedirectsToLogin(t, tampered, srv)

	// Expired: a validly-signed session token whose exp is in the past.
	expiredClaims := sessionClaims{
		UserID:  admin.ID,
		IsAdmin: true,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * panelSessionTTL)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	expiredToken, err := security.SignJWT(h.jwtSecret, expiredClaims)
	if err != nil {
		t.Fatalf("signing expired session: %v", err)
	}
	expiredClient := newPanelClient(t)
	setRawSessionCookie(t, expiredClient, srv, expiredToken)
	assertRedirectsToLogin(t, expiredClient, srv)
}

func setRawSessionCookie(t *testing.T, client *http.Client, srv *httptest.Server, value string) {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parsing server URL: %v", err)
	}
	//nolint:gosec // G124: seeding the test client's jar directly to simulate a tampered/expired cookie, not a server response
	client.Jar.SetCookies(u, []*http.Cookie{{Name: sessionCookieName, Value: value, Path: "/"}})
}

func assertRedirectsToLogin(t *testing.T, client *http.Client, srv *httptest.Server) {
	t.Helper()
	resp, err := client.Get(srv.URL + "/admin/panel")
	if err != nil {
		t.Fatalf("GET /admin/panel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/admin/panel/login" {
		t.Fatalf("expected redirect to login, got %d Location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// TestPanelSessionRevalidatesLiveAdminStatus proves requireAdminSession
// checks the database on every request instead of trusting the
// session cookie's IsAdmin claim: a demote applied out-of-band (as
// another admin would, from their own session) must take effect on the
// demoted admin's very next request, even though their cookie is still
// unexpired and still (falsely) claims IsAdmin=true.
func TestPanelSessionRevalidatesLiveAdminStatus(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin-a@example.com", "password-a-123")
	targetAdmin := mustCreatePanelAdmin(t, authStore, "admin-b@example.com", "password-b-123")

	client := newPanelClient(t)
	loginToPanel(t, client, srv, "admin-b@example.com", "password-b-123")

	// Sanity: the session works before the demote.
	resp, err := client.Get(srv.URL + "/admin/panel")
	if err != nil {
		t.Fatalf("GET /admin/panel before demote: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 before demote, got %d", resp.StatusCode)
	}

	// Another admin demotes targetAdmin out-of-band (directly through
	// the store, standing in for a second browser session).
	if err := authStore.updateUserAdmin(context.Background(), targetAdmin.ID, false); err != nil {
		t.Fatalf("updateUserAdmin: %v", err)
	}

	assertRedirectsToLogin(t, client, srv)
}

func TestPanelUserAndDeviceLifecycle(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")

	client := newPanelClient(t)
	loginToPanel(t, client, srv, "admin@example.com", "correct-horse-battery")

	csrfFor := func(path string) string {
		body, _ := getAndScrapeCSRF(t, client, srv, path)
		return scrapeCSRF(t, body)
	}

	// Create a second (non-admin) user from the panel.
	resp := postForm(t, client, srv, "/admin/panel/users", url.Values{
		"csrf_token": {csrfFor("/admin/panel")},
		"email":      {"member@example.com"},
		"password":   {"member-pass-123"},
	})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("create user: expected 302, got %d", resp.StatusCode)
	}

	member, err := authStore.getUserByEmail(context.Background(), "member@example.com")
	if err != nil {
		t.Fatalf("expected the panel-created user to exist: %v", err)
	}
	if member.IsAdmin {
		t.Fatalf("expected the new user to be created as non-admin")
	}

	// Prove both surfaces (panel + JSON API) share the same rows: the
	// panel-created user can log in through the normal JSON /auth/login.
	loginResp, err := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"member@example.com","password":"member-pass-123","device_name":"member-laptop"}`))
	if err != nil {
		t.Fatalf("JSON login as panel-created user: %v", err)
	}
	_ = loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected the panel-created user to log in via the JSON API, got %d", loginResp.StatusCode)
	}

	// Promote, then demote.
	resp = postForm(t, client, srv, "/admin/panel/users/"+member.ID+"/promote", url.Values{"csrf_token": {csrfFor("/admin/panel")}})
	_ = resp.Body.Close()
	member, _ = authStore.getUserByID(context.Background(), member.ID)
	if !member.IsAdmin {
		t.Fatalf("expected promote to set IsAdmin=true")
	}

	resp = postForm(t, client, srv, "/admin/panel/users/"+member.ID+"/demote", url.Values{"csrf_token": {csrfFor("/admin/panel")}})
	_ = resp.Body.Close()
	member, _ = authStore.getUserByID(context.Background(), member.ID)
	if member.IsAdmin {
		t.Fatalf("expected demote to set IsAdmin=false")
	}

	// The sole remaining admin (the seed admin) cannot be demoted or
	// deleted through the panel.
	seedAdmin, err := authStore.getUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("getUserByEmail seed admin: %v", err)
	}
	resp = postForm(t, client, srv, "/admin/panel/users/"+seedAdmin.ID+"/demote", url.Values{"csrf_token": {csrfFor("/admin/panel")}})
	_ = resp.Body.Close()
	if !strings.Contains(resp.Header.Get("Location"), "error=") {
		t.Fatalf("expected demoting the last admin to redirect with an error, got Location=%q", resp.Header.Get("Location"))
	}
	seedAdmin, _ = authStore.getUserByID(context.Background(), seedAdmin.ID)
	if !seedAdmin.IsAdmin {
		t.Fatalf("expected the rejected demote to leave the seed admin untouched")
	}

	// Soft-delete member: they lose the ability to log in anywhere, and
	// any devices they had are revoked.
	resp = postForm(t, client, srv, "/admin/panel/users/"+member.ID+"/delete", url.Values{"csrf_token": {csrfFor("/admin/panel")}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("delete user: expected 302, got %d", resp.StatusCode)
	}

	deniedLogin, err := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"member@example.com","password":"member-pass-123","device_name":"member-laptop"}`))
	if err != nil {
		t.Fatalf("post-delete login attempt: %v", err)
	}
	_ = deniedLogin.Body.Close()
	if deniedLogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected a soft-deleted user to be unable to log in via the API, got %d", deniedLogin.StatusCode)
	}

	// Restore: login works again.
	resp = postForm(t, client, srv, "/admin/panel/users/"+member.ID+"/restore", url.Values{"csrf_token": {csrfFor("/admin/panel")}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("restore user: expected 302, got %d", resp.StatusCode)
	}
	restoredLogin, err := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"member@example.com","password":"member-pass-123","device_name":"member-laptop-2"}`))
	if err != nil {
		t.Fatalf("post-restore login attempt: %v", err)
	}
	_ = restoredLogin.Body.Close()
	if restoredLogin.StatusCode != http.StatusOK {
		t.Fatalf("expected a restored user to be able to log in again, got %d", restoredLogin.StatusCode)
	}

	// Reset password: old password stops working, new one works, and
	// their device (created by the login above) is revoked.
	devicesBefore, err := authStore.listDevices(context.Background(), member.ID)
	if err != nil {
		t.Fatalf("listDevices before reset: %v", err)
	}
	if len(devicesBefore) == 0 {
		t.Fatalf("expected member to have at least one device before the password reset")
	}

	resp = postForm(t, client, srv, "/admin/panel/users/"+member.ID+"/reset-password",
		url.Values{"csrf_token": {csrfFor("/admin/panel")}, "password": {"brand-new-password-456"}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("reset password: expected 302, got %d", resp.StatusCode)
	}

	devicesAfter, err := authStore.listDevices(context.Background(), member.ID)
	if err != nil {
		t.Fatalf("listDevices after reset: %v", err)
	}
	if len(devicesAfter) != 0 {
		t.Fatalf("expected the password reset to revoke all of the user's devices, got %+v", devicesAfter)
	}

	oldPassLogin, _ := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"member@example.com","password":"member-pass-123","device_name":"x"}`))
	_ = oldPassLogin.Body.Close()
	if oldPassLogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected the old password to stop working after reset, got %d", oldPassLogin.StatusCode)
	}
	newPassLogin, _ := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"member@example.com","password":"brand-new-password-456","device_name":"x"}`))
	_ = newPassLogin.Body.Close()
	if newPassLogin.StatusCode != http.StatusOK {
		t.Fatalf("expected the new password to work after reset, got %d", newPassLogin.StatusCode)
	}

	// Devices page: revoke the device created by the successful new-
	// password login above, from the cross-user devices view.
	allDevices, err := authStore.listAllDevices(context.Background())
	if err != nil {
		t.Fatalf("listAllDevices: %v", err)
	}
	var toRevoke *deviceWithOwner
	for i := range allDevices {
		if allDevices[i].UserID == member.ID {
			toRevoke = &allDevices[i]
			break
		}
	}
	if toRevoke == nil {
		t.Fatalf("expected to find member's device in listAllDevices, got %+v", allDevices)
	}

	resp = postForm(t, client, srv, "/admin/panel/devices/"+toRevoke.ID+"/revoke",
		url.Values{"csrf_token": {csrfFor("/admin/panel/devices")}})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("revoke device: expected 302, got %d", resp.StatusCode)
	}

	remaining, err := authStore.listAllDevices(context.Background())
	if err != nil {
		t.Fatalf("listAllDevices after revoke: %v", err)
	}
	for _, d := range remaining {
		if d.ID == toRevoke.ID {
			t.Fatalf("expected device %s to be revoked", toRevoke.ID)
		}
	}
}
