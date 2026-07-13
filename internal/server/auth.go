package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
)

// Session handling. The private chamber (actions, detail, constitution) sits
// behind an entry splash; a visitor "enters" either by signing with a real
// Cardano wallet (CIP-30) or, as a demo fallback, by picking one of the body's
// members.
//
// The session is a cookie carrying the entering identity, signed with an
// HMAC-SHA256 key the server holds. The signature is what makes the identity
// trustworthy: without it a visitor could simply set the cookie to any
// delegate's name and vote as them. The cookie is readable but not forgeable.

const sessionCookie = "cella_member"

// csrfField is the form field carrying the anti-CSRF token on state-changing
// posts. SameSite=Lax already blocks the cross-site form post, but it is a
// browser-side control; the token is the server-side one.
const csrfField = "csrf"

// newKey returns the session-signing key: CELLA_SECRET when set, otherwise a
// random key generated at startup. A random key is safe but ephemeral —
// sessions do not survive a restart — so a persistent deployment should set
// CELLA_SECRET.
func newKey(secret string) []byte {
	if secret != "" {
		sum := sha256.Sum256([]byte(secret))
		return sum[:]
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		// crypto/rand failing is not a condition we can serve through: without a
		// key we cannot sign sessions, and an unsigned session is a forgeable one.
		panic("cella: cannot read random session key: " + err.Error())
	}
	return k
}

// sign returns the base64 HMAC of msg under the server key.
func (s *Server) sign(msg string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// openPaths are reachable without a session (the splash, auth endpoints,
// static/health). Everything else is gated.
func isOpenPath(p string) bool {
	if p == "/enter" || p == "/healthz" || p == "/logout" {
		return true
	}
	return strings.HasPrefix(p, "/auth/") || strings.HasPrefix(p, "/fonts/")
}

// member returns the identity in the current session. The second result is
// false when there is no cookie, or when its signature does not verify — a
// tampered or unsigned cookie is treated as no session at all.
func (s *Server) member(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	name, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(name)
	if err != nil {
		return "", false
	}
	identity := string(raw)
	if !hmac.Equal([]byte(sig), []byte(s.sign(identity))) {
		return "", false
	}
	return identity, true
}

// setMember writes the signed session cookie.
func (s *Server) setMember(w http.ResponseWriter, identity string) {
	name := base64.RawURLEncoding.EncodeToString([]byte(identity))
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    name + "." + s.sign(identity),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   12 * 60 * 60,
	})
}

// csrfToken derives the anti-CSRF token for a session. It is bound to the
// identity, so a token minted for one delegate cannot authorize a post as
// another, and it needs no server-side storage.
func (s *Server) csrfToken(identity string) string {
	return s.sign("csrf\x00" + identity)
}

// checkCSRF reports whether the request carries the right token for identity.
func (s *Server) checkCSRF(r *http.Request, identity string) bool {
	got := r.FormValue(csrfField)
	return got != "" && hmac.Equal([]byte(got), []byte(s.csrfToken(identity)))
}

// gate redirects unauthenticated visitors to the entry splash before serving
// any private route.
func (s *Server) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isOpenPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := s.member(r); !ok {
			http.Redirect(w, r, "/enter", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleMemberLogin is the demo fallback: enter as one of the roster members,
// with no proof of identity at all. It is the weakest door in the building —
// it hands a visitor a session as whichever delegate they name — so it exists
// only when the operator has explicitly asked for a demo. Refusing here is the
// control that matters: hiding the picker in the template would still leave the
// endpoint open to anyone who posts to it directly.
func (s *Server) handleMemberLogin(w http.ResponseWriter, r *http.Request) {
	if !s.demo {
		http.Error(w, "roster sign-in is disabled; authenticate with a wallet", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.FormValue("member")
	var found *Member
	for i := range demoBody.Members {
		if demoBody.Members[i].Name == name {
			found = &demoBody.Members[i]
			break
		}
	}
	if found == nil {
		http.Redirect(w, r, "/enter", http.StatusFound)
		return
	}
	s.setMember(w, found.Name+" (demo)")
	http.Redirect(w, r, "/", http.StatusFound)
}

// handleLogout clears the session and returns to the splash.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/enter", http.StatusFound)
}
