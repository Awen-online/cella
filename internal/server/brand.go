package server

import (
	"embed"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// The body's own mark is served by Cella, never hot-linked.
//
// Two reasons, and both matter. The Content-Security-Policy is img-src 'self' —
// deliberately, because a page that can load images from anywhere can leak which
// governance actions a committee is reading to whoever hosts them. And a
// self-hostable governance tool should not go dark because someone else's
// WordPress is down.
//
// So: the default mark is embedded in the binary, and an operator with their own
// can point CELLA_LOGO at a file on disk.

//go:embed brand/*.svg
var brandFS embed.FS

// logoPath is the file CELLA_LOGO named, or "" to serve the embedded mark.
var logoPath string

// SetLogo points the /brand/logo route at a file on disk.
func SetLogo(path string) { logoPath = path }

// handleBrand serves the body's mark.
func (s *Server) handleBrand(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/logo") {
		http.NotFound(w, r)
		return
	}

	// An operator's own mark, read from disk.
	if logoPath != "" {
		f, err := os.Open(logoPath)
		if err != nil {
			http.Error(w, "logo not readable", http.StatusInternalServerError)
			return
		}
		defer f.Close()

		w.Header().Set("Content-Type", contentTypeFor(logoPath))
		w.Header().Set("Cache-Control", "public, max-age=3600")
		stat, err := f.Stat()
		if err != nil {
			http.Error(w, "logo not readable", http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, filepath.Base(logoPath), stat.ModTime(), f)
		return
	}

	b, err := brandFS.ReadFile("brand/body-logo.svg")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

// contentTypeFor guesses from the extension. A logo is one of a handful of
// things and none of them need sniffing.
func contentTypeFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
