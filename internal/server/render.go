package server

import (
	"bytes"
	"html/template"
	"strings"

	"github.com/yuin/goldmark"
)

var mdRenderer = goldmark.New()

// mdHTML renders a governance abstract / metadata field (which may contain
// Markdown) to safe HTML. goldmark escapes raw HTML by default, so this is safe
// for anchored, third-party metadata.
func mdHTML(s string) template.HTML {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := mdRenderer.Convert([]byte(s), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(s))
	}
	return template.HTML(buf.String())
}
