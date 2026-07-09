package server

import (
	"embed"
	"net/http"
	"strings"
)

// Brand fonts (Cinzel, EB Garamond, JetBrains Mono) are OFL/open and embedded
// so the binary stays single-file and nothing loads from a third-party CDN —
// the type matches the Cella brand sheet exactly, self-hosted.
//
//go:embed fonts/*.woff2
var fontFS embed.FS

// handleFonts serves an embedded woff2 by name.
func (s *Server) handleFonts(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/fonts/")
	if name == "" || strings.Contains(name, "/") || !strings.HasSuffix(name, ".woff2") {
		http.NotFound(w, r)
		return
	}
	b, err := fontFS.ReadFile("fonts/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "font/woff2")
	w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	_, _ = w.Write(b)
}

// fontFace is the @font-face block prepended to every page's stylesheet so the
// brand font-family names resolve to the embedded files.
const fontFace = `
  @font-face{font-family:'Cinzel';font-weight:700;font-style:normal;font-display:swap;src:url('/fonts/cinzel-normal-700.woff2') format('woff2');}
  @font-face{font-family:'Cinzel';font-weight:800;font-style:normal;font-display:swap;src:url('/fonts/cinzel-normal-800.woff2') format('woff2');}
  @font-face{font-family:'EB Garamond';font-weight:400;font-style:normal;font-display:swap;src:url('/fonts/ebgaramond-normal-400.woff2') format('woff2');}
  @font-face{font-family:'EB Garamond';font-weight:400;font-style:italic;font-display:swap;src:url('/fonts/ebgaramond-italic-400.woff2') format('woff2');}
  @font-face{font-family:'JetBrains Mono';font-weight:400;font-style:normal;font-display:swap;src:url('/fonts/jetbrainsmono-normal-400.woff2') format('woff2');}
`

// withFonts injects the @font-face block right after a template's opening
// <style> tag. It's applied before parse, so it's static (unescaped) CSS.
func withFonts(h string) string {
	return strings.Replace(h, "<style>", "<style>"+fontFace, 1)
}
