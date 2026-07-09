package server

import (
	"html/template"
	"net/http"

	"github.com/Awen-online/cella/internal/constitution"
)

type constPage struct {
	Body     template.HTML
	Active   string
	Label    string
	Versions []constitution.Version
}

// handleConstitution serves the Cardano Constitution, the yardstick against
// which the committee judges every governance action.
func (s *Server) handleConstitution(w http.ResponseWriter, r *http.Request) {
	body, v, err := constitution.HTML(r.URL.Query().Get("v"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.ctpl.Execute(w, constPage{Body: body, Active: v.Key, Label: v.Label, Versions: constitution.Versions}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// constHTML is the Constitution reader (Cella-branded, versioned).
const constHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cardano Constitution ({{.Label}}) — Cella</title>
<style>
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#d7ddef; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --blue:#6f93ff; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--forum); color:var(--body); font-family:'EB Garamond',Georgia,serif; }
  header { padding:34px 6vw 18px; border-bottom:1px solid rgba(201,137,42,.25); }
  header .name { font-family:'Cinzel',serif; font-weight:800; letter-spacing:.06em; color:var(--ivory); font-size:24px; }
  header .name b { color:var(--gold); }
  header a.back { color:var(--blue); text-decoration:none; font-size:14px; }
  .bar { padding:14px 6vw 0; display:flex; align-items:center; gap:10px; flex-wrap:wrap; }
  .bar .lbl { font-family:'Cinzel',serif; color:var(--gold); font-size:11px; letter-spacing:.12em; text-transform:uppercase; }
  .bar a { font-size:13px; text-decoration:none; color:var(--muted); border:1px solid rgba(201,137,42,.3); border-radius:999px; padding:3px 12px; }
  .bar a.on { color:var(--forum); background:var(--goldb); border-color:var(--goldb); font-weight:600; }
  main { padding:14px 6vw 70px; }
  article.doc { max-width:820px; margin:0 auto; font-size:17px; line-height:1.62; }
  article.doc h1 { font-family:'Cinzel',serif; color:var(--ivory); font-weight:800; font-size:26px; letter-spacing:.03em; margin:26px 0 10px; }
  article.doc h2 { font-family:'Cinzel',serif; color:var(--goldb); font-weight:700; font-size:19px; letter-spacing:.04em; text-transform:uppercase; margin:30px 0 8px; padding-bottom:6px; border-bottom:1px solid rgba(201,137,42,.2); }
  article.doc h3 { font-family:'Cinzel',serif; color:var(--gold); font-weight:700; font-size:16px; margin:22px 0 6px; }
  article.doc p { margin:10px 0; }
  article.doc ol, article.doc ul { padding-left:26px; margin:10px 0; }
  article.doc li { margin:6px 0; }
  article.doc a { color:var(--blue); }
  article.doc hr { border:0; border-top:1px solid rgba(201,137,42,.2); margin:26px 0; }
  article.doc blockquote { border-left:3px solid var(--gold); margin:14px 0; padding:2px 16px; color:var(--muted); }
  article.doc table { border-collapse:collapse; width:100%; margin:14px 0; }
  article.doc th, article.doc td { border:1px solid rgba(201,137,42,.2); padding:8px 10px; text-align:left; font-size:15px; }
  footer { padding:20px 6vw; color:var(--muted); font-size:13px; border-top:1px solid rgba(201,137,42,.15); }
</style>
</head>
<body>
<header>
  <div class="name">CE<b>LL</b>A</div>
  <a class="back" href="/">← Governance actions</a>
</header>
<div class="bar">
  <span class="lbl">Constitution revision</span>
  {{range .Versions}}<a href="/constitution?v={{.Key}}" class="{{if eq .Key $.Active}}on{{end}}">{{.Label}}</a>{{end}}
</div>
<main>
  <article class="doc">{{.Body}}</article>
</main>
<footer>The Cardano Constitution is the yardstick for every Constitutional Committee vote. Cella · Apache-2.0</footer>
</body>
</html>`
