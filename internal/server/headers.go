package server

import "net/http"

// Cella's pages carry a delegate's authenticated session and a button that
// records a position into a permanent, on-chain-anchored record. That makes the
// browser's own defences worth turning on explicitly rather than relying on
// defaults.
//
// The Content-Security-Policy is strict because it can afford to be: Cella
// serves no third-party assets. Everything — styles, scripts, fonts — is
// same-origin or inline, so nothing legitimate needs a wider policy. The one
// concession is 'unsafe-inline', which the inline <style> and <script> blocks
// in the templates require; tightening that would mean hashing or noncing every
// block, which is worth doing but is not a header change.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		// A vote is one click. Framing the chamber inside a hostile page and
		// tricking a delegate into that click is exactly the attack to shut out.
		h.Set("X-Frame-Options", "DENY")

		// Same intent, stated to browsers that prefer CSP, plus a default-deny
		// on everything Cella has no reason to load.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"form-action 'self'; "+
				"base-uri 'none'; "+
				"frame-ancestors 'none'")

		// Do not let a browser second-guess a Content-Type we set deliberately.
		h.Set("X-Content-Type-Options", "nosniff")

		// A governance action's URL names what the committee is deliberating on.
		// It should not leak to whatever a delegate clicks through to.
		h.Set("Referrer-Policy", "no-referrer")

		next.ServeHTTP(w, r)
	})
}

// isTLS reports whether the request reached us over HTTPS, either directly or
// through a reverse proxy that terminated TLS and said so.
//
// Self-hosted Cella instances commonly sit behind nginx or Caddy, so trusting
// only r.TLS would leave the session cookie without its Secure flag on exactly
// the deployments that do have HTTPS.
func isTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}
