package server

import "net/http"

// The body's mark is served by Cella, never hot-linked.
//
// Two reasons, and both matter. The Content-Security-Policy is img-src 'self' —
// deliberately, because a page that can load images from anywhere can leak which
// governance actions a committee is reading to whoever hosts them. And a
// self-hostable governance tool should not go dark because someone else's server
// did.
//
// The mark travels with the body: it is a file sitting next to the body.json
// that names it, read at startup and held in memory. There is nothing to
// configure separately and nothing to keep in sync.
func (s *Server) handleBrand(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/brand/logo" || !s.body.HasLogo() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", s.body.LogoMIME)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(s.body.LogoData)
}
