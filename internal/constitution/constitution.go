// Package constitution embeds the versioned Cardano Constitution — the
// reference the Constitutional Committee judges every governance action
// against. It is shared by the web view (rendered HTML) and by the LLM review
// (raw text used to ground the model's assessment), so the binary stays
// self-contained with no external files to ship.
package constitution

import (
	"bytes"
	"embed"
	"html/template"

	"github.com/yuin/goldmark"
)

//go:embed data/*.md
var files embed.FS

// Version is one published revision of the Constitution.
type Version struct {
	Key     string // ?v= / lookup key
	Label   string // display label
	File    string // embedded path
	Current bool
}

// Versions lists revisions newest-first; the Current one is the default.
var Versions = []Version{
	{Key: "v2.4", Label: "v2.4 · current", File: "data/v2.4-current.md", Current: true},
	{Key: "v1", Label: "v1", File: "data/v1.md"},
	{Key: "v0", Label: "v0 · interim", File: "data/v0-interim.md"},
}

// resolve returns the Version for key, defaulting to the current revision when
// key is empty or unknown.
func resolve(key string) Version {
	for _, v := range Versions {
		if v.Key == key {
			return v
		}
	}
	for _, v := range Versions {
		if v.Current {
			return v
		}
	}
	return Versions[0]
}

// Text returns the raw markdown for key (default current) and the resolved
// revision. Used to ground the LLM review.
func Text(key string) (string, Version, error) {
	v := resolve(key)
	b, err := files.ReadFile(v.File)
	if err != nil {
		return "", v, err
	}
	return string(b), v, nil
}

var (
	md        = goldmark.New()
	htmlCache = map[string]template.HTML{}
)

// HTML returns the rendered (and cached) HTML for key and the resolved revision.
func HTML(key string) (template.HTML, Version, error) {
	v := resolve(key)
	if h, ok := htmlCache[v.Key]; ok {
		return h, v, nil
	}
	b, err := files.ReadFile(v.File)
	if err != nil {
		return "", v, err
	}
	var buf bytes.Buffer
	if err := md.Convert(b, &buf); err != nil {
		return "", v, err
	}
	// Trusted, embedded content — safe to render as HTML.
	h := template.HTML(buf.String()) //nolint:gosec
	htmlCache[v.Key] = h
	return h, v, nil
}
