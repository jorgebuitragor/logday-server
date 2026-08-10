package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Round-trip of the panel-specific session claims. Generic JWT signing/
// parsing/expiry behavior is covered in internal/security, which this
// wraps — see also TestPanelSessionRedirectsOnTamperedOrExpiredCookie
// in panel_handlers_test.go for the HTTP-level expiry/tamper behavior.
func TestPanelSessionRoundTrip(t *testing.T) {
	secret := []byte("test-secret")

	token, err := issuePanelSession(secret, "user-1", true, 24*time.Hour)
	if err != nil {
		t.Fatalf("issuePanelSession: %v", err)
	}

	c, err := parsePanelSession(secret, token)
	if err != nil {
		t.Fatalf("parsePanelSession: %v", err)
	}
	if c.UserID != "user-1" || !c.IsAdmin {
		t.Fatalf("unexpected claims: %+v", c)
	}
}

func TestVerifyCSRFMatchAndMismatch(t *testing.T) {
	newRequest := func(cookieValue, formValue string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/admin/panel/login", nil)
		if cookieValue != "" {
			//nolint:gosec // G124: simulating an incoming request cookie a browser would send, not a server response cookie
			r.AddCookie(&http.Cookie{Name: csrfCookieName, Value: cookieValue})
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if formValue != "" {
			r.Form.Set(csrfFormField, formValue)
		}
		return r
	}

	if !verifyCSRF(newRequest("token-123", "token-123")) {
		t.Fatalf("expected matching cookie/form CSRF tokens to verify")
	}
	if verifyCSRF(newRequest("token-123", "token-456")) {
		t.Fatalf("expected mismatched CSRF tokens to fail verification")
	}
	if verifyCSRF(newRequest("", "token-123")) {
		t.Fatalf("expected a missing CSRF cookie to fail verification")
	}
	if verifyCSRF(newRequest("token-123", "")) {
		t.Fatalf("expected a missing CSRF form field to fail verification")
	}
}
