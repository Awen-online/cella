package server

import (
	"net/http"
	"net/url"
	"strings"
)

// Session gating for the demo. The private chamber (actions, detail,
// constitution) sits behind an entry splash; a visitor "enters" either by
// signing with a real Cardano wallet (CIP-30, wired in a later pass) or, as a
// demo fallback, by picking one of the body's members. The session is a simple
// cookie holding the entering identity — sufficient for a demo, not a hardened
// auth system.

const sessionCookie = "cella_member"

// openPaths are reachable without a session (the splash, auth endpoints,
// static/health). Everything else is gated.
func isOpenPath(p string) bool {
	if p == "/enter" || p == "/healthz" || p == "/logout" {
		return true
	}
	return strings.HasPrefix(p, "/auth/") || strings.HasPrefix(p, "/fonts/")
}

// member returns the identity in the current session, if any.
func (s *Server) member(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	v, err := url.QueryUnescape(c.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

// setMember writes the session cookie.
func (s *Server) setMember(w http.ResponseWriter, identity string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    url.QueryEscape(identity),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   12 * 60 * 60,
	})
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

// handleMemberLogin is the demo fallback: enter as one of the roster members.
func (s *Server) handleMemberLogin(w http.ResponseWriter, r *http.Request) {
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
