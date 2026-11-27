package auth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jorgebuitragor/logday-server/internal/security"
)

// Handler side of specs/limites-auth: the 429 carries Retry-After, and
// a password over security.MaxPasswordLength never reaches argon2id.

func postLogin(t *testing.T, srv string, email, password string) *http.Response {
	t.Helper()
	resp, err := http.Post(srv+"/auth/login", "application/json",
		strings.NewReader(`{"email":"`+email+`","password":"`+password+`","device_name":"limits-test"}`))
	if err != nil {
		t.Fatalf("POST /auth/login: %v", err)
	}
	_ = resp.Body.Close()
	return resp
}

func TestLoginTooManyAttemptsSetsRetryAfter(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")

	for i := 0; i < maxAttemptsForTest; i++ {
		if resp := postLogin(t, srv.URL, "admin@example.com", "wrong-password"); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i+1, resp.StatusCode)
		}
	}

	resp := postLogin(t, srv.URL, "admin@example.com", "correct-horse-battery")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want 429 even with the right password, got %d", resp.StatusCode)
	}
	secs, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || secs < 1 || secs > int(windowForTest.Seconds()) {
		t.Fatalf("Retry-After = %q, want 1..60 seconds", resp.Header.Get("Retry-After"))
	}
}

func TestLoginRejectsOverlongPasswordAsInvalidCredentials(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")

	long := strings.Repeat("a", security.MaxPasswordLength+1)
	if resp := postLogin(t, srv.URL, "admin@example.com", long); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
	// It counts as a failure like any other: the limit hits one attempt sooner.
	for i := 1; i < maxAttemptsForTest; i++ {
		postLogin(t, srv.URL, "admin@example.com", "wrong-password")
	}
	if resp := postLogin(t, srv.URL, "admin@example.com", "correct-horse-battery"); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", resp.StatusCode)
	}
}

func TestAdminCreateUserRejectsOverlongPassword(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")

	loginResp, err := http.Post(srv.URL+"/auth/login", "application/json",
		strings.NewReader(`{"email":"admin@example.com","password":"correct-horse-battery","device_name":"api-test"}`))
	if err != nil {
		t.Fatalf("JSON login: %v", err)
	}
	body, _ := io.ReadAll(loginResp.Body)
	_ = loginResp.Body.Close()
	token := regexp.MustCompile(`"access_token":"([^"]+)"`).FindStringSubmatch(string(body))
	if token == nil {
		t.Fatalf("expected an access_token in the login response, got:\n%s", body)
	}

	long := strings.Repeat("a", security.MaxPasswordLength+1)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/users",
		strings.NewReader(`{"email":"member@example.com","password":"`+long+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token[1])
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /admin/users: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "at most") {
		t.Fatalf("want 400 'at most', got %d %q", resp.StatusCode, b)
	}
	if _, err := authStore.getUserByEmail(context.Background(), "member@example.com"); err == nil {
		t.Fatal("no user should be created")
	}
}

func TestPanelLoginTooManyAttemptsSetsRetryAfter(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")
	client := newPanelClient(t)

	try := func(password string) *http.Response {
		body, _ := getAndScrapeCSRF(t, client, srv, "/admin/panel/login")
		resp := postForm(t, client, srv, "/admin/panel/login", url.Values{
			"csrf_token": {scrapeCSRF(t, body)},
			"email":      {"admin@example.com"},
			"password":   {password},
		})
		_ = resp.Body.Close()
		return resp
	}

	// An overlong password fails like a wrong one and counts toward the limit.
	if resp := try(strings.Repeat("a", security.MaxPasswordLength+1)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("overlong password: want 401, got %d", resp.StatusCode)
	}
	for i := 1; i < maxAttemptsForTest; i++ {
		if resp := try("wrong-password"); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i+1, resp.StatusCode)
		}
	}

	resp := try("correct-horse-battery")
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("want a Retry-After header")
	}
}

func TestPanelPasswordFormsRejectOverlongPassword(t *testing.T) {
	long := strings.Repeat("a", security.MaxPasswordLength+1)

	t.Run("setup", func(t *testing.T) {
		srv, _, authStore := setupPanelServer(t)
		client := newPanelClient(t)
		body, _ := getAndScrapeCSRF(t, client, srv, "/setup")
		resp := postForm(t, client, srv, "/setup", url.Values{
			"csrf_token":       {scrapeCSRF(t, body)},
			"email":            {"admin@example.com"},
			"password":         {long},
			"password_confirm": {long},
		})
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(b), "como máximo") {
			t.Fatalf("want 400 with the length message, got %d", resp.StatusCode)
		}
		if n, _ := authStore.countUsers(context.Background()); n != 0 {
			t.Fatalf("want no user created, got %d", n)
		}
	})

	t.Run("create user and reset password", func(t *testing.T) {
		srv, _, authStore := setupPanelServer(t)
		mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")
		member := mustCreatePanelAdmin(t, authStore, "member@example.com", "member-pass-123")
		client := newPanelClient(t)
		loginToPanel(t, client, srv, "admin@example.com", "correct-horse-battery")
		csrfFor := func() string {
			body, _ := getAndScrapeCSRF(t, client, srv, "/admin/panel")
			return scrapeCSRF(t, body)
		}

		resp := postForm(t, client, srv, "/admin/panel/users", url.Values{
			"csrf_token": {csrfFor()},
			"email":      {"new@example.com"},
			"password":   {long},
		})
		_ = resp.Body.Close()
		if !strings.Contains(resp.Header.Get("Location"), "error=") {
			t.Fatalf("create: want an error redirect, got Location=%q", resp.Header.Get("Location"))
		}
		if _, err := authStore.getUserByEmail(context.Background(), "new@example.com"); err == nil {
			t.Fatal("create: no user should be created")
		}

		resp = postForm(t, client, srv, "/admin/panel/users/"+member.ID+"/reset-password", url.Values{
			"csrf_token": {csrfFor()},
			"password":   {long},
		})
		_ = resp.Body.Close()
		if !strings.Contains(resp.Header.Get("Location"), "error=") {
			t.Fatalf("reset: want an error redirect, got Location=%q", resp.Header.Get("Location"))
		}
		loginToPanel(t, newPanelClient(t), srv, "member@example.com", "member-pass-123")
	})
}

func TestLoginRecordsRealClientIP(t *testing.T) {
	srv, _, authStore := setupPanelServer(t)
	mustCreatePanelAdmin(t, authStore, "admin@example.com", "correct-horse-battery")
	cfg, err := security.ParseClientIPConfig("127.0.0.1, ::1", "")
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = security.ClientIPMiddleware(cfg)(srv.Config.Handler)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/auth/login",
		strings.NewReader(`{"email":"admin@example.com","password":"correct-horse-battery","device_name":"proxied"}`))
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /auth/login: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var ip string
	if err := authStore.db.QueryRow(`SELECT last_ip FROM devices WHERE device_name = 'proxied'`).Scan(&ip); err != nil {
		t.Fatalf("reading device ip: %v", err)
	}
	if ip != "203.0.113.9" {
		t.Fatalf("device ip = %q, want the client's", ip)
	}
}
