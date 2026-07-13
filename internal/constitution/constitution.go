// Package constitution embeds the versioned Cardano Constitution — the
// reference the Constitutional Committee judges every governance action
// against. It is shared by the web view (rendered HTML, with a navigable table
// of contents) and by the LLM review (raw text used to ground the model's
// assessment), so the binary stays self-contained with no external files to
// ship.
package constitution

import (
	"bytes"
	"embed"
	"html/template"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
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

// Entry is one heading in the table of contents.
//
// Both articles and their sections are collected. That matters more than it
// looks: the interim revision writes its articles as level-3 headings while the
// current one writes them as level-2, so a table of contents that gathered only
// level-2 headings would come out empty on v0.
type Entry struct {
	Level int    // 2 or 3
	Text  string // the heading, as written
	ID    string // the anchor to link to
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

// md renders with automatic heading IDs, so every article and section has an
// anchor. Without them the Constitution is a wall of text nobody can cite: a
// committee whose rationale says "contrary to Article III" should be able to
// link the reader to Article III.
var md = goldmark.New(
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

// rendered is one revision, parsed once. The documents are ~70 KB of markdown
// and never change at runtime, so re-parsing them per request is pure waste.
type rendered struct {
	HTML template.HTML
	TOC  []Entry
}

var (
	cacheMu sync.RWMutex
	cache   = map[string]rendered{}
)

// HTML returns the rendered document, its table of contents, and the resolved
// revision.
func HTML(key string) (template.HTML, []Entry, Version, error) {
	v := resolve(key)

	cacheMu.RLock()
	hit, ok := cache[v.Key]
	cacheMu.RUnlock()
	if ok {
		return hit.HTML, hit.TOC, v, nil
	}

	src, err := files.ReadFile(v.File)
	if err != nil {
		return "", nil, v, err
	}

	// Parse once and use the same tree for both the table of contents and the
	// rendered HTML, so every anchor in the one is guaranteed to exist in the
	// other.
	doc := md.Parser().Parse(text.NewReader(src))
	toc := tableOfContents(doc, src)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return "", nil, v, err
	}

	// Trusted, embedded content — safe to render as HTML.
	out := rendered{HTML: template.HTML(buf.String()), TOC: toc} //nolint:gosec

	cacheMu.Lock()
	cache[v.Key] = out
	cacheMu.Unlock()

	return out.HTML, out.TOC, v, nil
}

// tableOfContents collects the document's articles and their sections.
func tableOfContents(doc ast.Node, src []byte) []Entry {
	var toc []Entry

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		// Level 1 is the document's own title; it needs no entry.
		if h.Level < 2 || h.Level > 3 {
			return ast.WalkContinue, nil
		}
		raw, ok := h.AttributeString("id")
		if !ok {
			return ast.WalkContinue, nil
		}
		toc = append(toc, Entry{
			Level: h.Level,
			Text:  string(h.Text(src)),
			ID:    anchorOf(raw),
		})
		return ast.WalkContinue, nil
	})
	return toc
}

// anchorOf normalises goldmark's id attribute, which arrives as []byte.
func anchorOf(v any) string {
	switch s := v.(type) {
	case []byte:
		return string(s)
	case string:
		return s
	default:
		return ""
	}
}
