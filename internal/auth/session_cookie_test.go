package auth

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testAdminLogin = `{"email":"admin@example.com","password":"test-password-123","device_name":"Logday Web"}`

// browser is a client with its own cookie jar, like a browser tab.
func newBrowser(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func postAuth(t *testing.T, client *http.Client, srv *httptest.Server, path, body string, cookieModeOn bool, extra map[string]string) (*http.Response, tokenResponse) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookieModeOn {
		req.Header.Set(sessionModeHeader, sessionModeCookie)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	var tok tokenResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
			t.Fatalf("decoding %s response: %v", path, err)
		}
	}
	return resp, tok
}

func refreshCookieIn(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == refreshCookieName {
			return c
		}
	}
	return nil
}

func jarRefreshCookie(t *testing.T, client *http.Client, srv *httptest.Server) string {
	t.Helper()
	u, _ := url.Parse(srv.URL + "/auth/refresh")
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == refreshCookieName {
			return c.Value
		}
	}
	return ""
}

func TestCookieLoginKeepsRefreshTokenOutOfTheBody(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)

	resp, tok := postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: status %d", resp.StatusCode)
	}
	if tok.RefreshToken != "" {
		t.Fatalf("refresh_token leaked in the body in cookie mode")
	}
	if tok.AccessToken == "" {
		t.Fatalf("missing access token")
	}
	c := refreshCookieIn(resp)
	if c == nil {
		t.Fatalf("no %s cookie set", refreshCookieName)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != refreshCookiePath || c.Secure {
		t.Fatalf("cookie attributes = HttpOnly:%v SameSite:%v Path:%q Secure:%v, want HttpOnly, Strict, /auth, not Secure over plain HTTP", c.HttpOnly, c.SameSite, c.Path, c.Secure)
	}
}

func TestBodyLoginIsUnchanged(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)

	resp, tok := postAuth(t, newBrowser(t), srv, "/auth/login", testAdminLogin, false, nil)
	if resp.StatusCode != http.StatusOK || tok.RefreshToken == "" {
		t.Fatalf("body login: status %d, refresh_token present: %v", resp.StatusCode, tok.RefreshToken != "")
	}
	if refreshCookieIn(resp) != nil {
		t.Fatalf("body-mode login must not set a cookie")
	}
}

func TestCookieRefreshRotatesTheCookie(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)
	postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)
	before := jarRefreshCookie(t, browser, srv)

	resp, tok := postAuth(t, browser, srv, "/auth/refresh", "", true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: status %d", resp.StatusCode)
	}
	if tok.AccessToken == "" || tok.RefreshToken != "" {
		t.Fatalf("refresh body: access present %v, refresh present %v", tok.AccessToken != "", tok.RefreshToken != "")
	}
	if after := jarRefreshCookie(t, browser, srv); after == "" || after == before {
		t.Fatalf("cookie was not rotated")
	}
}

// Without the custom header the cookie must be ignored: that header is
// what a cross-site page can't add (CSRF guard).
func TestCookieIsIgnoredWithoutTheHeader(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)
	postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)

	resp, _ := postAuth(t, browser, srv, "/auth/refresh", "", false, nil)
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("refresh without %s used the cookie", sessionModeHeader)
	}
	resp, _ = postAuth(t, browser, srv, "/auth/refresh", "{}", false, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("refresh without header or token: status %d, want 400", resp.StatusCode)
	}
}

func TestCookieRefreshWithoutCookieIsNoSession(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)

	resp, _ := postAuth(t, newBrowser(t), srv, "/auth/refresh", "", true, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", resp.StatusCode)
	}
}

// A browser that still has a refresh token in localStorage from before
// cookie mode sends it once in the body; from then on it lives in the
// cookie.
func TestCookieRefreshMigratesABodyToken(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)
	_, legacy := postAuth(t, browser, srv, "/auth/login", testAdminLogin, false, nil)

	resp, tok := postAuth(t, browser, srv, "/auth/refresh", `{"refresh_token":"`+legacy.RefreshToken+`"}`, true, nil)
	if resp.StatusCode != http.StatusOK || tok.RefreshToken != "" {
		t.Fatalf("migration refresh: status %d, refresh in body %v", resp.StatusCode, tok.RefreshToken != "")
	}
	if jarRefreshCookie(t, browser, srv) == "" {
		t.Fatalf("migration did not set the cookie")
	}
	resp, _ = postAuth(t, browser, srv, "/auth/refresh", "", true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh with the migrated cookie: status %d", resp.StatusCode)
	}
}

// Reusing an already-rotated refresh token still revokes the device,
// and in cookie mode the dead cookie is cleared.
func TestCookieReuseRevokesAndClears(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)
	postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)
	stolen := jarRefreshCookie(t, browser, srv)
	postAuth(t, browser, srv, "/auth/refresh", "", true, nil) // the real tab rotates it

	attacker := newBrowser(t)
	resp, _ := postAuth(t, attacker, srv, "/auth/refresh", `{"refresh_token":"`+stolen+`"}`, true, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reused token: status %d, want 401", resp.StatusCode)
	}
	if c := refreshCookieIn(resp); c == nil || c.MaxAge >= 0 {
		t.Fatalf("reused token: cookie not cleared")
	}
	resp, _ = postAuth(t, browser, srv, "/auth/refresh", "", true, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after reuse the device should be revoked, refresh got %d", resp.StatusCode)
	}
}

func TestCookieLogoutRevokesAndClears(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)
	postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)
	token := jarRefreshCookie(t, browser, srv)

	resp, _ := postAuth(t, browser, srv, "/auth/logout", "", true, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: status %d", resp.StatusCode)
	}
	if c := refreshCookieIn(resp); c == nil || c.MaxAge >= 0 {
		t.Fatalf("logout did not clear the cookie")
	}
	if jarRefreshCookie(t, browser, srv) != "" {
		t.Fatalf("cookie still in the jar after logout")
	}
	// The device is gone, not just the cookie: the token is dead even
	// if someone kept a copy.
	resp, _ = postAuth(t, newBrowser(t), srv, "/auth/refresh", `{"refresh_token":"`+token+`"}`, false, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: status %d, want 401", resp.StatusCode)
	}
	// Logging out again is not an error.
	resp, _ = postAuth(t, browser, srv, "/auth/logout", "", true, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second logout: status %d", resp.StatusCode)
	}
}

func TestBodyLogoutRevokesTheDevice(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	client := newBrowser(t)
	_, tok := postAuth(t, client, srv, "/auth/login", testAdminLogin, false, nil)

	resp, _ := postAuth(t, client, srv, "/auth/logout", `{"refresh_token":"`+tok.RefreshToken+`"}`, false, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: status %d", resp.StatusCode)
	}
	if refreshCookieIn(resp) != nil {
		t.Fatalf("body-mode logout must not touch cookies")
	}
	resp, _ = postAuth(t, client, srv, "/auth/refresh", `{"refresh_token":"`+tok.RefreshToken+`"}`, false, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: status %d, want 401", resp.StatusCode)
	}
}

func TestCookieIsSecureBehindAnHTTPSProxy(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)

	resp, _ := postAuth(t, newBrowser(t), srv, "/auth/login", testAdminLogin, true, map[string]string{"X-Forwarded-Proto": "https"})
	if c := refreshCookieIn(resp); c == nil || !c.Secure {
		t.Fatalf("cookie should be Secure when the proxy says HTTPS")
	}
}

// Two refreshes racing with the same token (two tabs, or an attacker
// racing the real client): exactly one wins, the rest get 401 — never
// a 500 — and, as with any reuse, the device ends up revoked.
func TestConcurrentRefreshWithTheSameTokenIsReuse(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	_, tok := postAuth(t, newBrowser(t), srv, "/auth/login", testAdminLogin, false, nil)

	const n = 5
	type result struct {
		status int
		token  string
	}
	results := make(chan result, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			<-start
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/auth/refresh", strings.NewReader(`{"refresh_token":"`+tok.RefreshToken+`"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- result{status: -1}
				return
			}
			defer func() { _ = resp.Body.Close() }()
			var body tokenResponse
			if resp.StatusCode == http.StatusOK {
				_ = json.NewDecoder(resp.Body).Decode(&body)
			}
			results <- result{status: resp.StatusCode, token: body.RefreshToken}
		}()
	}
	close(start)

	var ok, unauthorized int
	var winner string
	for i := 0; i < n; i++ {
		r := <-results
		switch r.status {
		case http.StatusOK:
			ok++
			winner = r.token
		case http.StatusUnauthorized:
			unauthorized++
		default:
			t.Fatalf("concurrent refresh answered %d, want only 200 or 401", r.status)
		}
	}
	if ok != 1 || unauthorized != n-1 {
		t.Fatalf("got %d OK and %d 401, want exactly 1 OK", ok, unauthorized)
	}
	resp, _ := postAuth(t, newBrowser(t), srv, "/auth/refresh", `{"refresh_token":"`+winner+`"}`, false, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after a reuse race the device should be revoked, winner's token got %d", resp.StatusCode)
	}
}
