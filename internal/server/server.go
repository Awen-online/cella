// Package server serves Cella's minimal web UI: a list of ingested governance
// actions. It is intentionally dependency-free (net/http + html/template).
package server

import (
	"html/template"
	"net/http"
	"time"

	"github.com/Awen-online/cella/internal/store"
)

// Server is Cella's HTTP server.
type Server struct {
	db  *store.DB
	mux *http.ServeMux
	tpl *template.Template
}

// New builds a Server backed by db.
func New(db *store.DB) *Server {
	s := &Server{
		db:  db,
		mux: http.NewServeMux(),
		tpl: template.Must(template.New("index").Funcs(funcs).Parse(indexHTML)),
	}
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	return s
}

// ListenAndServe starts the server on addr.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	actions, err := s.db.Actions(100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.Execute(w, actions); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var funcs = template.FuncMap{
	"date": func(ts int64) string {
		if ts == 0 {
			return "—"
		}
		return time.Unix(ts, 0).UTC().Format("2006-01-02")
	},
}

// indexHTML is Cella-branded (forum navy + gold leaf + Cardano blue).
const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cella — Governance Actions</title>
<style>
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#cfd6ec; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --blue:#4d78ff; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--forum); color:var(--body); font-family:'EB Garamond',Georgia,serif; }
  header { padding:34px 6vw 18px; border-bottom:1px solid rgba(201,137,42,.25); }
  header .name { font-family:'Cinzel',serif; font-weight:800; letter-spacing:.06em; color:var(--ivory); font-size:30px; }
  header .name b { color:var(--gold); }
  header .tag { color:var(--muted); font-size:15px; margin-top:4px; }
  main { padding:24px 6vw 60px; }
  h2 { font-family:'Cinzel',serif; color:var(--ivory); font-weight:700; font-size:20px; letter-spacing:.04em; }
  table { width:100%; border-collapse:collapse; margin-top:14px; }
  th,td { text-align:left; padding:11px 12px; border-bottom:1px solid rgba(201,137,42,.15); vertical-align:top; }
  th { font-family:'Cinzel',serif; color:var(--gold); font-size:12px; letter-spacing:.12em; text-transform:uppercase; }
  td.type { color:var(--goldb); white-space:nowrap; font-size:14px; }
  td.title { color:var(--ivory); }
  td.id { font-family:ui-monospace,Consolas,monospace; font-size:12px; color:var(--muted); word-break:break-all; }
  td a { color:var(--blue); text-decoration:none; }
  .empty { margin-top:20px; padding:22px; border:1px dashed rgba(201,137,42,.35); border-radius:12px; color:var(--muted); }
  .empty code { color:var(--goldb); }
  footer { padding:20px 6vw; color:var(--muted); font-size:13px; border-top:1px solid rgba(201,137,42,.15); }
</style>
</head>
<body>
<header>
  <div class="name">CE<b>LL</b>A</div>
  <div class="tag">Self-hostable Cardano Constitutional Committee governance</div>
</header>
<main>
  <h2>Governance actions</h2>
  {{if .}}
  <table>
    <thead><tr><th>Date</th><th>Type</th><th>Title</th><th>Action ID</th></tr></thead>
    <tbody>
      {{range .}}
      <tr>
        <td>{{date .BlockTime}}</td>
        <td class="type">{{.Type}}</td>
        <td class="title">{{if .Title}}{{.Title}}{{else}}<span style="color:var(--muted)">(no anchored title)</span>{{end}}
          {{if .MetaURL}}<div style="margin-top:3px"><a href="{{.MetaURL}}" rel="noopener">rationale anchor ↗</a></div>{{end}}
        </td>
        <td class="id">{{.ProposalID}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{else}}
  <div class="empty">No governance actions yet. Run <code>cella ingest</code> to pull them from Koios.</div>
  {{end}}
</main>
<footer>Cella · built &amp; maintained by Awen LLC · Apache-2.0</footer>
</body>
</html>`
