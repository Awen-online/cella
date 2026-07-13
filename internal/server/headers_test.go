package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A vote is one click, and the chamber carries an authenticated session. Framing
// it inside a hostile page and tricking a delegate into that click is the attack
// these headers exist to shut out.
func TestSecurityHeaders(t *testing.T) {
	s, _ := seedServer(t)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	secureHeaders(s.gate(s.mux)).ServeHTTP(rec, r)

	want := map[string]string{
		"X-Frame-Options":        "DENY",
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
	}
	for h, v := range want {
		if got := rec.Header().Get(h); got != v {
			t.Errorf("%s = %q, want %q", h, got, v)
		}
	}

	csp := rec.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"default-src 'self'",
		"frame-ancestors 'none'", // clickjacking, stated to CSP-aware browsers
		"form-action 'self'",     // a vote cannot be posted to someone else's server
		"base-uri 'none'",
	} {
		if !strings.Contains(csp, directive) {
			t.Errorf("CSP is missing %q; got %q", directive, csp)
		}
	}
}

// Over HTTPS the session must not be sent back in the clear.
func TestSessionCookieIsSecureOverTLS(t *testing.T) {
	s, _ := seedServer(t)

	cases := map[string]struct {
		mutate     func(*http.Request)
		wantSecure bool
	}{
		"plain http": {
			func(*http.Request) {},
			false, // a Secure cookie would be dropped, locking out local `cella serve`
		},
		"direct TLS": {
			func(r *http.Request) { r.TLS = &tlsState },
			true,
		},
		"behind a TLS-terminating proxy": {
			func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") },
			true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			tc.mutate(r)

			rec := httptest.NewRecorder()
			s.setMember(rec, r, "Junia Marcia")

			cookies := rec.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("no cookie set")
			}
			c := cookies[0]
			if c.Secure != tc.wantSecure {
				t.Errorf("Secure = %v, want %v", c.Secure, tc.wantSecure)
			}
			// These hold regardless of transport.
			if !c.HttpOnly {
				t.Error("the session cookie is readable from JavaScript")
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("SameSite = %v, want Lax", c.SameSite)
			}
		})
	}
}

// A browser will not overwrite a Secure cookie with a non-Secure one, so the
// logout cookie must carry the same attributes or the session would survive it.
func TestLogoutCookieMatchesSessionAttributes(t *testing.T) {
	s, _ := seedServer(t)

	r := httptest.NewRequest(http.MethodGet, "/logout", nil)
	r.TLS = &tlsState
	rec := httptest.NewRecorder()
	s.handleLogout(rec, r)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("logout set no cookie")
	}
	c := cookies[0]
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want negative (delete the cookie)", c.MaxAge)
	}
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("the logout cookie does not match the session cookie's attributes: %+v", c)
	}
}

// tlsState is a minimal non-nil TLS connection state — its presence is what
// marks a request as having arrived over HTTPS.
var tlsState = tls.ConnectionState{HandshakeComplete: true}
