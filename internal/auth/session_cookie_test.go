package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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
	return cookieNamed(resp, plainRefreshCookieName)
}

func cookieNamed(resp *http.Response, name string) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func withReuseLeeway(t *testing.T, d time.Duration) {
	t.Helper()
	prev := refreshReuseLeeway
	refreshReuseLeeway = d
	t.Cleanup(func() { refreshReuseLeeway = prev })
}

func countDevices(t *testing.T, database *sql.DB) int {
	t.Helper()
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM devices`).Scan(&n); err != nil {
		t.Fatalf("counting devices: %v", err)
	}
	return n
}

func jarRefreshCookie(t *testing.T, client *http.Client, srv *httptest.Server) string {
	t.Helper()
	u, _ := url.Parse(srv.URL + "/auth/refresh")
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == plainRefreshCookieName {
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
		t.Fatalf("no %s cookie set", plainRefreshCookieName)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != plainRefreshCookiePath || c.Secure {
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
	withReuseLeeway(t, 0)
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
	c := cookieNamed(resp, secureRefreshCookieName)
	if c == nil || !c.Secure || c.Path != "/" || c.Domain != "" {
		t.Fatalf("over HTTPS the cookie should be __Host- prefixed, Secure, Path=/ and host-only, got %+v", c)
	}
	if cookieNamed(resp, plainRefreshCookieName) != nil {
		t.Fatalf("over HTTPS the unprefixed cookie must not be set")
	}
}

func TestCookieSecureAlwaysForcesSecure(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "always")
	srv, _, _ := setupPolicyTestServer(t)

	resp, _ := postAuth(t, newBrowser(t), srv, "/auth/login", testAdminLogin, true, nil)
	if c := cookieNamed(resp, secureRefreshCookieName); c == nil || !c.Secure {
		t.Fatalf("COOKIE_SECURE=always should set the Secure __Host- cookie even over plain HTTP")
	}
}

// Two refreshes racing with the same token (two tabs, or a reload while
// a refresh is in flight): within the reuse leeway every one of them
// succeeds — none gets a 500 and the session is not revoked.
func TestConcurrentRefreshWithinTheLeewayKeepsTheSession(t *testing.T) {
	srv, _, database := setupPolicyTestServer(t)
	_, tok := postAuth(t, newBrowser(t), srv, "/auth/login", testAdminLogin, false, nil)
	before := countDevices(t, database)

	const n = 5
	statuses := make(chan int, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			<-start
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/auth/refresh", strings.NewReader(`{"refresh_token":"`+tok.RefreshToken+`"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				statuses <- -1
				return
			}
			_ = resp.Body.Close()
			statuses <- resp.StatusCode
		}()
	}
	close(start)
	for i := 0; i < n; i++ {
		if s := <-statuses; s != http.StatusOK {
			t.Fatalf("concurrent refresh within the leeway answered %d, want 200", s)
		}
	}
	if after := countDevices(t, database); after != before {
		t.Fatalf("devices went from %d to %d: the session was revoked", before, after)
	}
}

// Within the leeway, presenting a just-rotated token rotates the
// device's current one instead of revoking: what a reload during an
// in-flight refresh looks like to the server.
func TestJustRotatedTokenWithinTheLeewayStillRefreshes(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)
	postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)
	lost := jarRefreshCookie(t, browser, srv)
	postAuth(t, browser, srv, "/auth/refresh", "", true, nil) // its Set-Cookie "never arrived"

	reloaded := newBrowser(t)
	resp, tok := postAuth(t, reloaded, srv, "/auth/refresh", `{"refresh_token":"`+lost+`"}`, true, nil)
	if resp.StatusCode != http.StatusOK || tok.AccessToken == "" {
		t.Fatalf("just-rotated token within the leeway: status %d", resp.StatusCode)
	}
	if resp, _ := postAuth(t, reloaded, srv, "/auth/refresh", "", true, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("the new cookie should keep working, got %d", resp.StatusCode)
	}
}

// Once the cookie exists it wins over a body token: a second tab that
// still had the pre-cookie token in localStorage, after the first tab
// migrated it, must not get the device revoked as reuse.
func TestCookieWinsOverABodyTokenOnceMigrated(t *testing.T) {
	withReuseLeeway(t, 0)
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)
	_, legacy := postAuth(t, browser, srv, "/auth/login", testAdminLogin, false, nil)
	body := `{"refresh_token":"` + legacy.RefreshToken + `"}`
	if resp, _ := postAuth(t, browser, srv, "/auth/refresh", body, true, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("first tab migration: status %d", resp.StatusCode)
	}

	resp, _ := postAuth(t, browser, srv, "/auth/refresh", body, true, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second tab with the old body token and the new cookie: status %d, want 200", resp.StatusCode)
	}
}

// Two cookies with our name means one was planted (a parent domain or
// another path): neither is trusted.
func TestDuplicateSessionCookiesAreNoSession(t *testing.T) {
	srv, _, _ := setupPolicyTestServer(t)
	browser := newBrowser(t)
	postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)
	real := jarRefreshCookie(t, browser, srv)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/auth/refresh", strings.NewReader(""))
	req.Header.Set(sessionModeHeader, sessionModeCookie)
	req.Header.Add("Cookie", plainRefreshCookieName+"=planted-by-a-sibling")
	req.Header.Add("Cookie", plainRefreshCookieName+"="+real)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("duplicate cookies: status %d, want 401", resp.StatusCode)
	}
}

// A refresh in flight when the user logs out may already have rotated
// the token the logout carries: the device must still be revoked.
func TestLogoutWithAnAlreadyRotatedTokenRevokes(t *testing.T) {
	srv, _, database := setupPolicyTestServer(t)
	before := countDevices(t, database)
	client := newBrowser(t)
	_, tok := postAuth(t, client, srv, "/auth/login", testAdminLogin, false, nil)
	postAuth(t, client, srv, "/auth/refresh", `{"refresh_token":"`+tok.RefreshToken+`"}`, false, nil)

	resp, _ := postAuth(t, client, srv, "/auth/logout", `{"refresh_token":"`+tok.RefreshToken+`"}`, false, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: status %d", resp.StatusCode)
	}
	if after := countDevices(t, database); after != before {
		t.Fatalf("devices: %d before login, %d after logout — the device survived", before, after)
	}
}

// Logging in again while the browser still holds a working session
// cookie (lost hint, failed restore) must not leave the old device
// orphaned next to the new one.
func TestCookieLoginReplacesTheOldCookieSession(t *testing.T) {
	srv, _, database := setupPolicyTestServer(t)
	browser := newBrowser(t)
	postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)
	afterFirst := countDevices(t, database)

	postAuth(t, browser, srv, "/auth/login", testAdminLogin, true, nil)
	if got := countDevices(t, database); got != afterFirst {
		t.Fatalf("devices after logging in again with a live cookie: %d, want %d", got, afterFirst)
	}
}
