// Package server serves Cella's minimal web UI: governance actions and the
// Constitutional Committee's votes and rationales. It is intentionally
// dependency-free (net/http + html/template).
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

// actionView is a governance action plus its Constitutional Committee votes and
// its (AI-assisted) constitutionality review.
type actionView struct {
	store.ActionRow
	Votes            []store.VoteRow
	Yes, No, Abstain int
	Review           store.ReviewRow
	HasReview        bool
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

	ids := make([]string, len(actions))
	for i, a := range actions {
		ids[i] = a.ProposalID
	}
	votes, err := s.db.VotesFor(ids)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reviews, err := s.db.ReviewsFor(ids)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	views := make([]actionView, 0, len(actions))
	for _, a := range actions {
		av := actionView{ActionRow: a, Votes: votes[a.ProposalID]}
		for _, v := range av.Votes {
			switch v.Vote {
			case "Yes":
				av.Yes++
			case "No":
				av.No++
			case "Abstain":
				av.Abstain++
			}
		}
		if rv, ok := reviews[a.ProposalID]; ok {
			av.Review, av.HasReview = rv, true
		}
		views = append(views, av)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.Execute(w, views); err != nil {
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
	"short": func(s string) string {
		if len(s) <= 16 {
			return s
		}
		return s[:8] + "…" + s[len(s)-6:]
	},
}

// indexHTML is Cella-branded (forum navy + gold leaf + Cardano blue).
const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Cella — Governance Actions &amp; CC Votes</title>
<style>
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#cfd6ec; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --blue:#4d78ff; --green:#4bbd88; --red:#d9695f; }
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
  td.id { font-family:ui-monospace,Consolas,monospace; font-size:12px; color:var(--muted); }
  td a { color:var(--blue); text-decoration:none; }
  .tally { font-size:13px; white-space:nowrap; }
  .tally .y { color:var(--green); } .tally .n { color:var(--red); } .tally .a { color:var(--muted); }
  .votes { margin-top:6px; }
  .votes .v { font-size:12.5px; margin:2px 0; }
  .votes .v b.y { color:var(--green); } .votes .v b.n { color:var(--red); } .votes .v b.a { color:var(--muted); }
  .votes .cc { font-family:ui-monospace,Consolas,monospace; color:var(--muted); font-size:11px; }
  .review { margin-top:8px; }
  .pill { display:inline-block; font-family:'Cinzel',serif; font-size:10px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; padding:2px 8px; border-radius:999px; border:1px solid; }
  .pill.constitutional { color:var(--green); border-color:rgba(75,189,136,.5); }
  .pill.unconstitutional { color:var(--red); border-color:rgba(217,105,95,.5); }
  .pill.uncertain { color:var(--goldb); border-color:rgba(245,210,122,.5); }
  .rsum { font-size:13px; color:var(--body); margin-top:4px; max-width:520px; }
  .legend { color:var(--muted); font-size:13px; margin-top:6px; font-style:italic; }
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
  <h2>Governance actions &amp; Constitutional Committee votes</h2>
  <div class="legend">Constitutionality tags are AI-assisted assessments — the committee decides and signs. Run <code>cella review</code> to generate them with your own model.</div>
  {{if .}}
  <table>
    <thead><tr><th>Date</th><th>Type</th><th>Action</th><th>CC votes &amp; rationales</th></tr></thead>
    <tbody>
      {{range .}}
      <tr>
        <td>{{date .BlockTime}}</td>
        <td class="type">{{.Type}}</td>
        <td class="title">
          {{if .Title}}{{.Title}}{{else}}<span style="color:var(--muted)">(no anchored title)</span>{{end}}
          <div class="id">{{short .ProposalID}}</div>
          {{if .HasReview}}
          <div class="review">
            <span class="pill {{.Review.Verdict}}">AI · {{.Review.Verdict}}</span>
            <div class="rsum">{{.Review.Summary}}</div>
          </div>
          {{end}}
        </td>
        <td>
          {{if .Votes}}
          <div class="tally"><b class="y">{{.Yes}} Yes</b> · <b class="n">{{.No}} No</b> · <b class="a">{{.Abstain}} Abstain</b></div>
          <div class="votes">
            {{range .Votes}}
            <div class="v">
              <b class="{{if eq .Vote "Yes"}}y{{else if eq .Vote "No"}}n{{else}}a{{end}}">{{.Vote}}</b>
              <span class="cc">{{short .VoterID}}</span>
              {{if .RationaleURL}}· <a href="{{.RationaleURL}}" rel="noopener">rationale ↗</a>{{end}}
            </div>
            {{end}}
          </div>
          {{else}}
          <span style="color:var(--muted)">no CC votes yet</span>
          {{end}}
        </td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{else}}
  <div class="empty">No governance actions yet. Run <code>cella ingest</code> to pull actions and CC votes from Koios.</div>
  {{end}}
</main>
<footer>Cella · built &amp; maintained by Awen LLC · Apache-2.0</footer>
</body>
</html>`
