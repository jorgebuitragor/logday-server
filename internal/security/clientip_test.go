package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseClientIPConfig(t *testing.T) {
	cfg, err := ParseClientIPConfig(" 10.0.1.0/24, 192.168.1.7 ,::ffff:172.16.0.1,fd00::/8,", "cf-connecting-ip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Header != "Cf-Connecting-Ip" {
		t.Errorf("header = %q, want the canonical form", cfg.Header)
	}
	want := []string{"10.0.1.0/24", "192.168.1.7/32", "172.16.0.1/32", "fd00::/8"}
	if len(cfg.TrustedProxies) != len(want) {
		t.Fatalf("got %v, want %v", cfg.TrustedProxies, want)
	}
	for i, p := range cfg.TrustedProxies {
		if p.String() != want[i] {
			t.Errorf("proxy %d = %s, want %s", i, p, want[i])
		}
	}

	empty, err := ParseClientIPConfig("", "")
	if err != nil || len(empty.TrustedProxies) != 0 || empty.Header != "X-Forwarded-For" {
		t.Fatalf("empty config = %+v, %v", empty, err)
	}

	for _, bad := range []string{"10.0.0.0/33", "not-an-ip", "10.0.0.1/abc"} {
		if _, err := ParseClientIPConfig(bad, ""); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestClientIP(t *testing.T) {
	xff, _ := ParseClientIPConfig("10.0.1.0/24, 172.16.0.0/12", "")
	cf, _ := ParseClientIPConfig("10.0.1.0/24", "CF-Connecting-IP")
	none, _ := ParseClientIPConfig("", "")

	cases := []struct {
		name    string
		cfg     ClientIPConfig
		remote  string
		headers map[string][]string
		want    string
	}{
		{"no trusted proxies ignores the header", none, "10.0.1.3:4000",
			map[string][]string{"X-Forwarded-For": {"203.0.113.9"}}, "10.0.1.3"},
		{"untrusted remote can't set the header", xff, "198.51.100.1:4000",
			map[string][]string{"X-Forwarded-For": {"203.0.113.9"}}, "198.51.100.1"},
		{"trusted proxy, one hop", xff, "10.0.1.3:4000",
			map[string][]string{"X-Forwarded-For": {"203.0.113.9"}}, "203.0.113.9"},
		{"client-written entries left of the real one are ignored", xff, "10.0.1.3:4000",
			map[string][]string{"X-Forwarded-For": {"1.2.3.4, 203.0.113.9"}}, "203.0.113.9"},
		{"skips trusted hops from the right", xff, "10.0.1.3:4000",
			map[string][]string{"X-Forwarded-For": {"203.0.113.9, 172.17.0.2"}}, "203.0.113.9"},
		{"several header lines", xff, "10.0.1.3:4000",
			map[string][]string{"X-Forwarded-For": {"1.2.3.4", "203.0.113.9, 172.17.0.2"}}, "203.0.113.9"},
		{"garbage stops the walk at the last good hop", xff, "10.0.1.3:4000",
			map[string][]string{"X-Forwarded-For": {"203.0.113.9, nonsense, 172.17.0.2"}}, "172.17.0.2"},
		{"all hops trusted gives the leftmost", xff, "10.0.1.3:4000",
			map[string][]string{"X-Forwarded-For": {"172.17.0.5, 172.17.0.2"}}, "172.17.0.5"},
		{"no header keeps the proxy", xff, "10.0.1.3:4000", nil, "10.0.1.3"},
		{"IPv4-mapped IPv6 is unmapped", xff, "[::ffff:10.0.1.3]:4000",
			map[string][]string{"X-Forwarded-For": {"::ffff:203.0.113.9"}}, "203.0.113.9"},
		{"single-value header", cf, "10.0.1.3:4000",
			map[string][]string{"Cf-Connecting-Ip": {" 2001:db8::1 "}, "X-Forwarded-For": {"162.158.1.1"}}, "2001:db8::1"},
		{"single-value header that isn't an IP keeps the proxy", cf, "10.0.1.3:4000",
			map[string][]string{"Cf-Connecting-Ip": {"1.2.3.4, 5.6.7.8"}}, "10.0.1.3"},
		{"single-value header from an untrusted remote", cf, "198.51.100.1:4000",
			map[string][]string{"Cf-Connecting-Ip": {"203.0.113.9"}}, "198.51.100.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for k, vs := range tc.headers {
				for _, v := range vs {
					r.Header.Add(k, v)
				}
			}
			if got := tc.cfg.ClientIP(r).String(); got != tc.want {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestClientIPMiddlewareRewritesRemoteAddr(t *testing.T) {
	cfg, _ := ParseClientIPConfig("10.0.1.0/24", "")

	var seen string
	h := ClientIPMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.1.3:4000"
	r.Header.Set("X-Forwarded-For", "2001:db8::7")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen != "[2001:db8::7]:4000" {
		t.Errorf("RemoteAddr = %q", seen)
	}

	// Without trusted proxies the request passes untouched.
	h = ClientIPMiddleware(ClientIPConfig{})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	}))
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.1.3:4000"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen != "10.0.1.3:4000" {
		t.Errorf("RemoteAddr = %q, want it untouched", seen)
	}

	// A RemoteAddr without a port still gets a valid host:port.
	h = ClientIPMiddleware(cfg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	}))
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.1.3"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen != "203.0.113.9:0" {
		t.Errorf("RemoteAddr = %q", seen)
	}
}
