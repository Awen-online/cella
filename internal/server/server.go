// Package server serves Cella's minimal web UI: governance actions and the
// Constitutional Committee's votes and rationales. It is intentionally
// dependency-free (net/http + html/template).
package server

import (
	"html/template"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Awen-online/cella/internal/govaction"
	"github.com/Awen-online/cella/internal/koios"
	"github.com/Awen-online/cella/internal/store"
)

// Options configures the server.
type Options struct {
	// Secret signs session cookies and CSRF tokens. Empty means a random key
	// generated at startup — secure, but sessions do not survive a restart.
	Secret string

	// Demo enables the entry splash's roster picker, which signs a visitor in as
	// any delegate with no proof of identity. Never enable it on a reachable
	// deployment.
	Demo bool

	// Body is the delegate roster. The zero value falls back to the placeholder
	// roster, whose addresses cannot authenticate anyone.
	Body Body
}

// Server is Cella's HTTP server.
type Server struct {
	db   *store.DB
	key  []byte // signs session cookies and CSRF tokens
	demo bool   // the roster picker is available (never in production)
	body Body   // the delegate roster
	mux  *http.ServeMux
	tpl  *template.Template
	dtpl *template.Template
	ctpl *template.Template
	etpl *template.Template
	stpl *template.Template
	rtpl *template.Template
}

// New builds a Server backed by db.
func New(db *store.DB, opts Options) *Server {
	body := opts.Body
	if len(body.Members) == 0 {
		body = demoBody
	}
	s := &Server{
		db:   db,
		key:  newKey(opts.Secret),
		demo: opts.Demo,
		body: body,
		mux:  http.NewServeMux(),
		tpl:  template.Must(template.New("index").Funcs(funcs).Parse(withFonts(indexHTML))),
		dtpl: template.Must(template.Must(template.Must(
			template.New("detail").Funcs(funcs).Parse(withFonts(detailHTML))).
			Parse(payloadHTML)).
			Parse(votingContextHTML)),
		ctpl: template.Must(template.New("constitution").Parse(withFonts(constHTML))),
		etpl: template.Must(template.New("enter").Parse(withFonts(enterHTML))),
		stpl: template.Must(template.New("submit").Parse(withFonts(submitHTML))),
		rtpl: template.Must(template.New("rationale").Funcs(funcs).Parse(withFonts(rationaleHTML))),
	}
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/fonts/", s.handleFonts)
	s.mux.HandleFunc("/action/", s.handleAction)
	s.mux.HandleFunc("/submit/", s.handleSubmit)
	s.mux.HandleFunc("/rationale/", s.handleRationale)
	s.mux.HandleFunc("/constitution", s.handleConstitution)
	s.mux.HandleFunc("/enter", s.handleEnter)
	s.mux.HandleFunc("/vote", s.handleCastVote)
	s.mux.HandleFunc("/vote/prepare", s.handleVotePrepare)
	s.mux.HandleFunc("/flag", s.handleFlag)
	s.mux.HandleFunc("/draft", s.handleDraft)
	s.mux.HandleFunc("/auth/member", s.handleMemberLogin)
	s.mux.HandleFunc("/auth/challenge", s.handleChallenge)
	s.mux.HandleFunc("/auth/verify", s.handleVerify)
	s.mux.HandleFunc("/logout", s.handleLogout)
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})
	return s
}

// ListenAndServe starts the server on addr. The private chamber sits behind the
// entry splash via the session gate.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           secureHeaders(s.gate(s.mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
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
	AbstractHTML     template.HTML

	// The chamber: the body's delegates and the positions they have recorded.
	BodyName        string
	Deliberation    []MemberStance
	Tally           tally
	ChamberPosition string

	// HasRationale is true once the committee's rationale has been authored.
	HasRationale bool

	// Deadline is when voting closes on this action. Zero when the action is
	// already settled — there is nothing left to count down to.
	Deadline Deadline

	// Payload is what the action actually does on-chain: the recipients of a
	// treasury withdrawal, the parameters a change would set, the version a hard
	// fork targets. Everything else about an action is what its authors said
	// about it; this is the binding part.
	Payload    govaction.Payload
	HasPayload bool

	// Summary is how the rest of the ecosystem is voting — stake-weighted DRep
	// and SPO tallies. Context for the committee's own decision, not a mandate.
	Summary    koios.VotingSummary
	HasSummary bool

	// The proposer's own case (CIP-108), as opposed to the committee's.
	MotivationHTML        template.HTML
	ProposerRationaleHTML template.HTML

	// Alignment names the Constitution articles that most directly govern this
	// kind of action, and links into them.
	Alignment    Alignment
	HasAlignment bool

	// The Constitutional Committee as the chain seats it, and the threshold it
	// must clear. YesNeeded is the quorum fraction of the authorized seats,
	// rounded up — with 7 seats and a 2/3 quorum it is 5, not 4, and a committee
	// that rounded down would believe it had ratified something the chain
	// rejects.
	Committee      []CommitteeSeat
	Seats          int
	YesNeeded      int
	QuorumFraction string
	ThresholdKnown bool
	ThresholdMet   bool

	// The signed-in delegate's own recorded position, which drives the ballot
	// form. YouRecorded is false until they have actually taken one.
	You           string
	YourVote      string
	YourRationale string
	YouRecorded   bool

	// YouCanSign is true when the signed-in delegate has a wallet registered in
	// the roster, and so can sign their position rather than merely assert it.
	YouCanSign bool

	// Flags are the chamber's coordination flags — a delegate raising a hand to
	// their colleagues. Draft is the signed-in delegate's own private notes,
	// which nobody else can read.
	Flags []FlagView
	Draft string

	// CSRF is the anti-forgery token for this session, embedded in every form
	// that changes state.
	CSRF string
}

// CommitteeSeat is one CC member's position on an action (or a pending seat).
type CommitteeSeat struct {
	Name       string
	Credential string
	TermEnds   int64 // epoch the seat's term expires
	Voted      bool
	Vote       string
	Rationale  string
}

// idxView is the data for the actions index: the signed-in member + the rows.
type idxView struct {
	Member  string
	Actions []actionView
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

	m, _ := s.member(r)
	you := strings.TrimSuffix(m, " (demo)")
	myVotes, err := s.db.MemberVotesByMember(you)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	chamberVotes, err := s.db.MemberVotesForAll(ids)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	net, err := s.db.Network()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now()

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
		if mv, ok := myVotes[a.ProposalID]; ok {
			av.YourVote = mv.Vote
		}
		av.Tally, _ = tallyFrom(chamberVotes[a.ProposalID], s.body)

		// The clock only means something while the action is still open. Running
		// a countdown on one the chain enacted last month is a lie, and running
		// one on an action already ratified invites a committee to vote on
		// something that is no longer theirs to decide.
		if a.Expiration.Valid && a.Live() {
			av.Deadline = deadlineFor(a.Expiration.Int64, net, now)
		}
		views = append(views, av)
	}

	// What is about to run out comes first. The chain hands actions to us newest
	// first, which is not the order a committee needs them in.
	slices.SortStableFunc(views, byUrgency)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.Execute(w, idxView{Member: m, Actions: views}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleAction renders a single governance action's detail page: metadata,
// the full CC vote roster with rationales, and the AI-assisted review.
func (s *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/action/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}

	a, ok, err := s.db.ActionBySlug(slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	votes, err := s.db.VotesFor([]string{a.ProposalID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	reviews, err := s.db.ReviewsFor([]string{a.ProposalID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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
	av.AbstractHTML = mdHTML(a.Abstract)

	// The chamber: every delegate's recorded position, and where that leaves the
	// body. Nothing here is inferred — a delegate who has not voted shows as
	// awaiting.
	av.BodyName = s.body.Name
	av.Deliberation, err = s.chamber(a.ProposalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, _, err := s.tallyFor(a.ProposalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	av.Tally = t
	av.ChamberPosition = t.position()

	sessionID, _ := s.member(r)
	you := strings.TrimSuffix(sessionID, " (demo)")
	av.You = you
	av.CSRF = s.csrfToken(sessionID)
	for _, st := range av.Deliberation {
		if st.Name == you {
			av.YourVote, av.YourRationale, av.YouRecorded = st.Vote, st.Rationale, st.Recorded
			break
		}
	}
	if m, ok := s.body.ByName(you); ok && m.Address != "" {
		av.YouCanSign = true
	}

	// The chamber's raised hands, and this delegate's own private notes.
	av.Flags, err = s.flagViews(a.ProposalID, you)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	av.Draft, _, err = s.db.Draft(a.ProposalID, you)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// A rationale can only be anchored once someone has written one.
	rat, _, err := s.db.RationaleFor(a.ProposalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	av.HasRationale = !rat.Empty()

	// The clock. An action that expires unvoted is an accidental abstention.
	net, err := s.db.Network()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if a.Expiration.Valid && a.Live() {
		av.Deadline = deadlineFor(a.Expiration.Int64, net, time.Now())
	}

	// What the action actually does, and how the rest of the ecosystem is
	// voting on it.
	av.Payload, av.HasPayload = a.Payload()
	av.Summary, av.HasSummary = a.Summary()
	av.MotivationHTML = mdHTML(a.Motivation)
	av.ProposerRationaleHTML = mdHTML(a.ProposerRationale)
	av.Alignment, av.HasAlignment = alignmentFor(a.Type)

	// The Constitutional Committee, as the chain currently seats it. Resigned
	// members are excluded: their vote does not count, and counting them in the
	// denominator would understate the threshold.
	ci, err := s.db.Committee()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	byCred := make(map[string]store.VoteRow, len(av.Votes))
	for _, v := range av.Votes {
		byCred[v.VoterID] = v
	}
	seen := map[string]bool{}
	for _, m := range ci.Authorized() {
		seat := CommitteeSeat{
			Name:       ccMemberName(m.HotID),
			Credential: m.HotID,
		}
		if m.ExpirationEpoch != nil {
			seat.TermEnds = *m.ExpirationEpoch
		}
		if v, ok := byCred[m.HotID]; ok {
			seat.Voted, seat.Vote, seat.Rationale = true, v.Vote, v.RationaleURL
		}
		seen[m.HotID] = true
		av.Committee = append(av.Committee, seat)
	}
	// A vote from a credential the roster does not know about is still a vote,
	// and hiding it would misreport the tally.
	for _, v := range av.Votes {
		if !seen[v.VoterID] {
			av.Committee = append(av.Committee, CommitteeSeat{
				Name: ccMemberName(v.VoterID), Credential: v.VoterID,
				Voted: true, Vote: v.Vote, Rationale: v.RationaleURL,
			})
		}
	}

	// The threshold the committee must clear, and whether it has.
	av.Seats = len(ci.Authorized())
	av.YesNeeded = ci.YesNeeded()
	av.QuorumFraction = ci.Quorum()
	av.ThresholdKnown = av.YesNeeded > 0
	av.ThresholdMet = av.ThresholdKnown && av.Yes >= av.YesNeeded

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.dtpl.Execute(w, av); err != nil {
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
	"ccname": ccMemberName,

	// ada renders lovelace exactly. A treasury figure that has been through a
	// float is a treasury figure that may be wrong.
	"ada": govaction.ADA,

	// pct/pctf give a recipient's share of a withdrawal, for the bar and the label.
	"pctf": func(part, total *big.Int) float64 {
		return govaction.Recipient{Lovelace: part}.Percent(total)
	},
	"pct": func(part, total *big.Int) string {
		p := govaction.Recipient{Lovelace: part}.Percent(total)
		// Keep a hairline visible for a recipient too small to see, so nobody is
		// invisible in the layout.
		if p > 0 && p < 0.4 {
			p = 0.4
		}
		return strconv.FormatFloat(p, 'f', 2, 64)
	},

	// depositADA renders the proposer's staked deposit.
	"depositADA": func(lovelace string) string {
		n, ok := new(big.Int).SetString(lovelace, 10)
		if !ok {
			return ""
		}
		return govaction.ADA(n)
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
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#cfd6ec; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --blue:#6f93ff; --green:#4bbd88; --red:#d9695f; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--forum); color:var(--body); font-family:'EB Garamond',Georgia,serif; }
  header { border-bottom:1px solid rgba(201,137,42,.25); }
  header .hin { max-width:1500px; margin:0 auto; padding:34px 40px 18px; }
  footer .fin { max-width:1500px; margin:0 auto; padding:20px 40px; }
  header .name { font-family:'Cinzel',serif; font-weight:800; letter-spacing:.06em; color:var(--ivory); font-size:30px; }
  header .name b { color:var(--gold); }
  header .topbar { display:flex; align-items:center; justify-content:space-between; gap:12px; }
  header .brand { display:flex; align-items:center; gap:13px; }
  header .badge { width:42px; height:42px; flex:0 0 auto; }
  header a.leave { color:var(--muted); text-decoration:none; font-family:'Cinzel',serif; font-size:11px; letter-spacing:.1em; text-transform:uppercase; white-space:nowrap; }
  header a.leave:hover { color:var(--gold); }
  header .who { display:flex; align-items:center; gap:14px; }
  header .whoami { color:var(--goldb); font-family:'Cinzel',serif; font-size:11px; letter-spacing:.1em; text-transform:uppercase; white-space:nowrap; }
  header .tag { color:var(--muted); font-size:15px; margin-top:4px; }
  header a.nav { display:inline-block; margin-top:10px; color:var(--goldb); text-decoration:none; font-family:'Cinzel',serif; font-size:12px; letter-spacing:.1em; text-transform:uppercase; border:1px solid rgba(245,210,122,.4); border-radius:999px; padding:4px 14px; }
  header a.nav:hover { background:rgba(245,210,122,.12); }
  main { padding:24px 40px 60px; max-width:1500px; margin:0 auto; }
  h2 { font-family:'Cinzel',serif; color:var(--ivory); font-weight:700; font-size:20px; letter-spacing:.04em; }
  table { width:100%; border-collapse:collapse; margin-top:14px; }
  th,td { text-align:left; padding:11px 12px; border-bottom:1px solid rgba(201,137,42,.15); vertical-align:top; }
  th { font-family:'Cinzel',serif; color:var(--gold); font-size:12px; letter-spacing:.12em; text-transform:uppercase; }
  td.type { color:var(--goldb); white-space:nowrap; font-size:14px; }
  td.title { color:var(--ivory); }
  td.dl { white-space:nowrap; }
  .cd { font-family:'Cinzel',serif; font-size:13px; font-weight:700; letter-spacing:.04em; }
  .cd.critical { color:var(--red); }
  .cd.soon { color:var(--goldb); }
  .cd.ok { color:var(--green); }
  .cd.expired { color:var(--muted); text-decoration:line-through; }
  .cd.unknown { color:var(--muted); font-weight:400; }
  .dlwhen { color:var(--muted); font-size:11.5px; margin-top:3px; }
  .stpill { display:inline-block; font-family:'Cinzel',serif; font-size:10px; font-weight:700; letter-spacing:.09em; text-transform:uppercase; padding:2px 10px; border-radius:999px; border:1px solid; }
  .stpill.Enacted { color:var(--green); border-color:rgba(75,189,136,.5); }
  .stpill.Ratified { color:var(--goldb); border-color:rgba(245,210,122,.5); }
  .stpill.Dropped, .stpill.Expired { color:var(--muted); border-color:rgba(139,147,184,.4); }
  td.quorum { white-space:nowrap; }
  .qn { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:14px; color:var(--muted); }
  .qn.part { color:var(--goldb); }
  .qn.full { color:var(--green); }
  .qlab { color:var(--muted); font-size:11px; margin-top:2px; }
  td .muted { color:var(--muted); }
  td.title a.atitle { color:var(--ivory); font-weight:600; text-decoration:none; }
  td.title a.atitle:hover { color:var(--goldb); text-decoration:underline; }
  td.id { font-family:ui-monospace,Consolas,monospace; font-size:12px; color:var(--muted); }
  td a { color:var(--blue); text-decoration:none; }
  .tally { font-size:13px; white-space:nowrap; }
  .tally .y { color:var(--green); } .tally .n { color:var(--red); } .tally .a { color:var(--muted); }
  .votes { margin-top:6px; }
  .votes .v { font-size:12.5px; margin:2px 0; }
  .votes .v b.y { color:var(--green); } .votes .v b.n { color:var(--red); } .votes .v b.a { color:var(--muted); }
  .myvote { white-space:nowrap; }
  .vpill { display:inline-block; font-family:'Cinzel',serif; font-size:11px; letter-spacing:.06em; text-transform:uppercase; font-weight:700; padding:3px 11px; border-radius:999px; border:1px solid; text-decoration:none; }
  .vpill.y { color:var(--green); border-color:rgba(75,189,136,.5); } .vpill.n { color:var(--red); border-color:rgba(217,105,95,.5); } .vpill.a { color:var(--muted); border-color:rgba(139,147,184,.4); }
  .vcast { font-size:12.5px; color:var(--goldb); text-decoration:none; border-bottom:1px dotted rgba(201,137,42,.5); white-space:nowrap; }
  .vcast:hover { color:var(--gold); }
  .votes .cc { font-family:ui-monospace,Consolas,monospace; color:var(--muted); font-size:11px; }
  .review { margin-top:8px; }
  .pill { display:inline-block; font-family:'Cinzel',serif; font-size:10px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; padding:2px 8px; border-radius:999px; border:1px solid; }
  .pill.constitutional { color:var(--green); border-color:rgba(75,189,136,.5); }
  .pill.unconstitutional { color:var(--red); border-color:rgba(217,105,95,.5); }
  .pill.uncertain { color:var(--goldb); border-color:rgba(245,210,122,.5); }
  .rsum { font-size:13px; color:var(--body); margin-top:4px; max-width:520px; }
  .legend { color:var(--muted); font-size:15px; margin-top:8px; font-style:italic; }
  .empty { margin-top:20px; padding:22px; border:1px dashed rgba(201,137,42,.35); border-radius:12px; color:var(--muted); }
  .empty code { color:var(--goldb); }
  footer { color:var(--muted); font-size:13px; border-top:1px solid rgba(201,137,42,.15); }
</style>
</head>
<body>
<header>
  <div class="hin">
  <div class="topbar">
    <div class="brand">
      <svg class="badge" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" role="img" aria-label="Cella"><rect width="100" height="100" rx="22" fill="#0A0E27"></rect><g transform="translate(18,16) scale(0.64)"><path d="M22 86 L22 42 A28 28 0 0 1 78 42 L78 86" fill="none" stroke="#FAF7EE" stroke-width="9"></path><rect x="11" y="84" width="78" height="9" rx="1.5" fill="#FAF7EE"></rect><circle cx="50" cy="62" r="6.5" fill="#F5D27A"></circle></g></svg>
      <span class="name">CE<b>LL</b>A</span>
    </div>
    <div class="who">{{if .Member}}<span class="whoami">Signed in as {{.Member}}</span>{{end}}<a class="leave" href="/logout">Sign out</a></div>
  </div>
  <div class="tag">Self-hostable Cardano Constitutional Committee governance</div>
  <a class="nav" href="/constitution">Read the Constitution →</a>
  </div>
</header>
<main>
  <h2>Governance actions &amp; Constitutional Committee votes</h2>
  <div class="legend">Constitutionality tags are AI-assisted assessments — the committee decides and signs. Run <code>cella review</code> to generate them with your own model.</div>
  {{if .Actions}}
  <table>
    <thead><tr><th>Deadline</th><th>Chamber</th><th>Type</th><th>Action</th><th>Your vote</th><th>CC votes &amp; rationales</th></tr></thead>
    <tbody>
      {{range .Actions}}
      <tr>
        <td class="dl">
          {{if .Settled}}
            <div class="stpill {{.Status}}">{{.Status}}</div>
            <div class="dlwhen">decided on-chain</div>
          {{else if .Deadline.Epoch}}
            <div class="cd {{.Deadline.Urgency}}" {{if .Deadline.Unix}}data-deadline="{{.Deadline.Unix}}"{{end}}>{{if .Deadline.Countdown}}{{.Deadline.Countdown}}{{else}}epoch {{.Deadline.Epoch}}{{end}}</div>
            <div class="dlwhen">{{.Deadline.When}}</div>
          {{else}}
            <span class="muted">—</span>
          {{end}}
        </td>
        <td class="quorum">
          <span class="qn {{if eq .Tally.Recorded .Tally.Seats}}full{{else if .Tally.Recorded}}part{{end}}">{{.Tally.Recorded}}/{{.Tally.Seats}}</span>
          <div class="qlab">recorded</div>
        </td>
        <td class="type">{{.Type}}</td>
        <td class="title">
          <a class="atitle" href="/action/{{.Slug}}">{{if .Title}}{{.Title}}{{else}}(no anchored title){{end}}</a>
          <div class="id">{{short .ProposalID}} · <a href="https://adastat.net/governances/{{.GovID}}" target="_blank" rel="noopener">AdaStat &#8599;</a></div>
          {{if .HasReview}}
          <div class="review">
            <span class="pill {{.Review.Verdict}}">AI · {{.Review.Verdict}}</span>
            <div class="rsum">{{.Review.Summary}}</div>
          </div>
          {{end}}
        </td>
        <td class="myvote">
          {{if .YourVote}}
          <a class="vpill {{if eq .YourVote "Yes"}}y{{else if eq .YourVote "No"}}n{{else}}a{{end}}" href="/action/{{.Slug}}#your-position">{{.YourVote}}</a>
          {{else}}
          <a class="vcast" href="/action/{{.Slug}}#your-position">Cast &rarr;</a>
          {{end}}
        </td>
        <td>
          {{if .Votes}}
          <div class="tally"><b class="y">{{.Yes}} Yes</b> · <b class="n">{{.No}} No</b> · <b class="a">{{.Abstain}} Abstain</b></div>
          <div class="votes">
            {{range .Votes}}
            <div class="v">
              <b class="{{if eq .Vote "Yes"}}y{{else if eq .Vote "No"}}n{{else}}a{{end}}">{{.Vote}}</b>
              <span class="cc">{{$n := ccname .VoterID}}{{if $n}}{{$n}}{{else}}{{short .VoterID}}{{end}}</span>
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
<footer><div class="fin">Cella · built &amp; maintained by Awen LLC · Apache-2.0</div></footer>
<script>
// Tick the countdowns so a page left open does not quietly go stale — a stale
// clock on a governance deadline is worse than no clock.
(function () {
  var els = Array.prototype.slice.call(document.querySelectorAll('.cd[data-deadline]'));
  if (!els.length) return;

  function plural(n, unit) { return n === 1 ? '1 ' + unit : n + ' ' + unit + 's'; }

  function render(el) {
    var left = (parseInt(el.getAttribute('data-deadline'), 10) * 1000) - Date.now();
    el.classList.remove('ok', 'soon', 'critical', 'expired');
    if (left <= 0) { el.textContent = 'expired'; el.classList.add('expired'); return; }

    var mins = left / 60000, hours = mins / 60, days = hours / 24;
    if (hours < 1)       el.textContent = plural(Math.ceil(mins), 'minute') + ' left';
    else if (hours < 48) el.textContent = plural(Math.floor(hours), 'hour') + ' left';
    else                 el.textContent = plural(Math.floor(days), 'day') + ' left';

    el.classList.add(hours < 48 ? 'critical' : (days < 5 ? 'soon' : 'ok'));
  }

  function tick() { els.forEach(render); }
  tick();
  setInterval(tick, 30000);
})();
</script>
</body>
</html>`

// detailHTML is a single governance action's detail page: metadata, abstract,
// AI-assisted review, and the full CC vote roster with rationales.
const detailHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{if .Title}}{{.Title}}{{else}}Governance action{{end}} — Cella</title>
<style>
  :root { --forum:#0A0E27; --veil:#131A40; --ivory:#FAF7EE; --body:#cfd6ec; --muted:#8b93b8; --gold:#C9892A; --goldb:#F5D27A; --blue:#6f93ff; --green:#4bbd88; --red:#d9695f; }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--forum); color:var(--body); font-family:'EB Garamond',Georgia,serif; }
  /* One centred column, so the page uses the desktop it is given instead of
     hugging the left edge. Borders stay full-bleed; only the content centres. */
  header { border-bottom:1px solid rgba(201,137,42,.25); }
  header .hin { max-width:1100px; margin:0 auto; padding:34px 40px 18px; }
  footer .fin { max-width:1100px; margin:0 auto; padding:20px 40px; }
  header .name { font-family:'Cinzel',serif; font-weight:800; letter-spacing:.06em; color:var(--ivory); font-size:24px; }
  header .name b { color:var(--gold); }
  header a.back { color:var(--blue); text-decoration:none; font-size:15.5px; }
  main { padding:24px 40px 60px; max-width:1100px; margin:0 auto; width:100%; }
  h1 { font-family:'Cinzel',serif; color:var(--ivory); font-weight:700; font-size:24px; letter-spacing:.02em; line-height:1.25; margin:6px 0 4px; }
  .type { color:var(--goldb); font-size:13px; text-transform:uppercase; letter-spacing:.08em; font-family:'Cinzel',serif; }
  .meta { color:var(--muted); font-size:15px; margin-top:8px; line-height:1.65; }
  .meta code { font-family:ui-monospace,Consolas,monospace; color:var(--body); font-size:12px; word-break:break-all; }
  .deadline { margin-top:12px; padding:10px 14px; border-radius:10px; border:1px solid; display:flex; align-items:baseline; gap:12px; flex-wrap:wrap; }
  .deadline .cd { font-family:'Cinzel',serif; font-size:15px; font-weight:700; letter-spacing:.04em; }
  .deadline .dlwhen { color:var(--muted); font-size:13px; }
  .deadline .dlnote { color:var(--muted); font-size:12.5px; font-style:italic; flex-basis:100%; }
  .deadline.critical { border-color:rgba(217,105,95,.5); background:rgba(217,105,95,.08); }
  .deadline.critical .cd { color:var(--red); }
  .deadline.soon { border-color:rgba(245,210,122,.45); background:rgba(245,210,122,.06); }
  .deadline.soon .cd { color:var(--goldb); }
  .deadline.ok { border-color:rgba(75,189,136,.35); background:rgba(75,189,136,.06); }
  .deadline.ok .cd { color:var(--green); }
  .deadline.expired { border-color:rgba(139,147,184,.3); }
  .deadline.expired .cd { color:var(--muted); }
  .deadline.unknown { border-color:rgba(139,147,184,.3); }
  .deadline.unknown .cd { color:var(--muted); font-weight:400; }

  /* Lifecycle: the chain has already decided. */
  .settled { margin-top:12px; padding:10px 14px; border-radius:10px; display:flex; align-items:baseline; gap:12px; flex-wrap:wrap; border:1px solid rgba(139,147,184,.35); background:rgba(139,147,184,.07); }
  .settled .st-badge { font-family:'Cinzel',serif; font-size:12px; font-weight:700; letter-spacing:.1em; text-transform:uppercase; padding:3px 12px; border-radius:999px; border:1px solid; }
  .settled .st-note { color:var(--muted); font-size:13.5px; }
  .settled.Enacted { border-color:rgba(75,189,136,.45); background:rgba(75,189,136,.07); }
  .settled.Enacted .st-badge { color:var(--green); border-color:rgba(75,189,136,.5); }
  .settled.Ratified .st-badge { color:var(--goldb); border-color:rgba(245,210,122,.5); }
  .settled.Dropped .st-badge, .settled.Expired .st-badge { color:var(--muted); border-color:rgba(139,147,184,.4); }

  /* The anchored metadata does not match what was signed on-chain. */
  .anchorbad { margin-top:12px; padding:11px 14px; border-radius:10px; border:1px solid rgba(217,105,95,.5); background:rgba(217,105,95,.08); color:#e8a49c; font-size:13.5px; line-height:1.55; }
  .anchorbad b { color:#f0b8b1; }

  /* On-chain payload. */
  .pl-head { font-size:16px; color:var(--body); margin-bottom:12px; }
  .pl-total { color:var(--goldb); font-size:19px; font-family:'Cinzel',serif; letter-spacing:.02em; }
  .pl-sub { font-family:'Cinzel',serif; color:var(--gold); font-size:11px; letter-spacing:.1em; text-transform:uppercase; margin:14px 0 7px; }
  .pl-rows { display:flex; flex-direction:column; gap:9px; }
  .pl-row { display:grid; grid-template-columns:130px minmax(0,1fr) 54px; gap:10px; align-items:center; }
  .pl-row > * { min-width:0; }
  .pl-amt { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:13px; color:var(--goldb); text-align:right; }
  .pl-barwrap { height:9px; background:rgba(139,147,184,.14); border-radius:999px; overflow:hidden; }
  .pl-bar { height:100%; background:linear-gradient(90deg,var(--gold),var(--goldb)); border-radius:999px; }
  .pl-pct { font-size:12px; color:var(--muted); text-align:right; }
  .pl-cred { grid-column:1 / -1; font-size:11.5px; color:var(--muted); word-break:break-all; margin-top:-3px; }
  .pl-cred code { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:11px; color:var(--body); }
  .pl-net { font-family:'Cinzel',serif; font-size:9.5px; letter-spacing:.08em; text-transform:uppercase; color:var(--muted); border:1px solid rgba(139,147,184,.3); border-radius:999px; padding:1px 7px; margin-right:6px; }
  .pl-script { color:var(--goldb); font-size:10px; }
  .pl-seat { font-size:12px; color:var(--body); word-break:break-all; }
  .pl-seat code { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:11px; color:var(--goldb); }
  .pl-seat.pl-removed code { color:var(--red); text-decoration:line-through; }
  .pl-params { width:100%; border-collapse:collapse; margin-top:4px; }
  .pl-params th { text-align:left; font-family:'Cinzel',serif; color:var(--gold); font-size:10.5px; letter-spacing:.1em; text-transform:uppercase; padding:6px 10px; border-bottom:1px solid rgba(201,137,42,.2); }
  .pl-params td { padding:8px 10px; border-bottom:1px solid rgba(201,137,42,.1); font-size:13.5px; vertical-align:top; }
  .pl-params code { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:12px; color:var(--body); }
  .pl-val { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:12.5px; color:var(--goldb); word-break:break-all; }
  .pl-kv { font-size:13.5px; color:var(--muted); margin:6px 0; word-break:break-all; }
  .pl-kv span { font-family:'Cinzel',serif; font-size:10px; letter-spacing:.08em; text-transform:uppercase; color:var(--gold); margin-right:8px; }
  .pl-kv code { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:11.5px; color:var(--body); }
  .pl-note { color:var(--muted); font-size:13px; line-height:1.55; margin-top:12px; font-style:italic; }
  .pl-note a { color:var(--blue); }
  .pl-foot { color:var(--muted); font-size:11.5px; margin-top:12px; word-break:break-all; }
  .pl-foot code { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; color:var(--body); }
  .pl-raw { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:11.5px; color:var(--body); background:var(--forum); border-radius:8px; padding:12px; overflow-x:auto; margin-top:10px; }

  /* How the rest of the chain is voting. */
  .vc-note { color:var(--muted); font-size:13.5px; line-height:1.55; margin-bottom:16px; }
  .vc-role { margin-bottom:18px; }
  .vc-name { font-family:'Cinzel',serif; color:var(--ivory); font-size:13px; letter-spacing:.03em; margin-bottom:7px; }
  .vc-n { color:var(--muted); font-family:'EB Garamond',serif; font-size:12.5px; letter-spacing:0; margin-left:8px; }
  .vc-bar { display:flex; height:11px; border-radius:999px; overflow:hidden; background:rgba(139,147,184,.14); }
  .vc-seg.y { background:var(--green); } .vc-seg.n { background:var(--red); }
  .vc-legend { margin-top:7px; font-size:13px; color:var(--muted); }
  .vc-legend .y { color:var(--green); } .vc-legend .n { color:var(--red); } .vc-legend .a { color:var(--muted); }
  .vc-cast { margin-left:8px; font-size:12px; }
  .vc-foot { color:var(--muted); font-size:12px; font-style:italic; border-top:1px solid rgba(201,137,42,.12); padding-top:11px; }

  /* The proposer's own case. */
  .pcase-note { color:var(--muted); font-size:13px; font-style:italic; margin-bottom:12px; }
  .pcase { margin:8px 0; }
  .pcase summary { cursor:pointer; font-family:'Cinzel',serif; color:var(--goldb); font-size:12px; letter-spacing:.05em; padding:6px 0; }
  .pcase summary:hover { color:var(--gold); }
  .meta a { color:var(--blue); text-decoration:none; }
  .card { background:var(--veil); border:1px solid rgba(201,137,42,.18); border-radius:12px; padding:18px 20px; margin-top:20px; min-width:0; overflow-wrap:anywhere; }
  .card h2 { font-family:'Cinzel',serif; color:var(--gold); font-size:13px; letter-spacing:.12em; text-transform:uppercase; margin:0 0 10px; }
  .abstract { color:var(--body); font-size:15.5px; line-height:1.6; }
  .abstract p { margin:0 0 10px; }
  .abstract ul, .abstract ol { padding-left:22px; margin:8px 0; }
  .abstract li { margin:4px 0; }
  .abstract a { color:var(--blue); }
  .abstract h1, .abstract h2, .abstract h3 { color:var(--ivory); font-family:'Cinzel',serif; font-size:16px; margin:16px 0 6px; }
  .abstract code { font-family:ui-monospace,Consolas,monospace; font-size:13px; color:var(--goldb); }
  .ccname { color:var(--goldb); font-family:'Cinzel',serif; font-size:12px; letter-spacing:.03em; }
  .pill { display:inline-block; font-family:'Cinzel',serif; font-size:10px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; padding:3px 10px; border-radius:999px; border:1px solid; }
  .pill.constitutional { color:var(--green); border-color:rgba(75,189,136,.5); }
  .pill.unconstitutional { color:var(--red); border-color:rgba(217,105,95,.5); }
  .pill.uncertain { color:var(--goldb); border-color:rgba(245,210,122,.5); }
  .rsum { font-size:15px; color:var(--body); margin-top:8px; line-height:1.5; }
  .rmodel { color:var(--muted); font-size:12px; margin-top:6px; font-style:italic; }
  .al-lead { font-size:15px; line-height:1.55; color:var(--body); }
  .al-arts { display:flex; flex-wrap:wrap; gap:9px; margin-top:12px; }
  .al-art { font-family:'Cinzel',serif; font-size:12px; letter-spacing:.04em; color:var(--goldb); text-decoration:none; border:1px solid rgba(245,210,122,.4); border-radius:999px; padding:6px 14px; }
  .al-art:hover { background:rgba(245,210,122,.1); }
  .al-note { color:var(--muted); font-size:12.5px; font-style:italic; margin-top:12px; line-height:1.5; }
  .tally { font-size:15px; margin-bottom:10px; }
  .tally .y { color:var(--green); } .tally .n { color:var(--red); } .tally .a { color:var(--muted); }
  table.votes { width:100%; border-collapse:collapse; table-layout:fixed; }
  table.votes th,table.votes td { text-align:left; padding:9px 10px; border-bottom:1px solid rgba(201,137,42,.12); font-size:13px; vertical-align:top; }
  table.votes th { font-family:'Cinzel',serif; color:var(--gold); font-size:11px; letter-spacing:.1em; text-transform:uppercase; }
  table.votes td.vote b.y { color:var(--green); } table.votes td.vote b.n { color:var(--red); } table.votes td.vote b.a { color:var(--muted); }
  table.votes td.cc { font-family:ui-monospace,Consolas,monospace; color:var(--muted); font-size:12px; word-break:break-all; }
  table.votes td a { color:var(--blue); text-decoration:none; }
  .muted { color:var(--muted); }
  .votes tr.pending { opacity:.42; }
  .await { color:var(--muted); font-family:'Cinzel',serif; font-size:11px; letter-spacing:.06em; text-transform:uppercase; }
  .seatsof { color:var(--muted); font-size:13px; }
  .thresh { margin-bottom:12px; padding:9px 13px; border-radius:9px; border:1px solid rgba(245,210,122,.4); background:rgba(245,210,122,.06); font-size:14px; color:var(--body); }
  .thresh b { color:var(--goldb); }
  .thresh.met { border-color:rgba(75,189,136,.45); background:rgba(75,189,136,.07); }
  .thresh.met b { color:var(--green); }
  .thresh.unknown { border-color:rgba(139,147,184,.3); background:none; color:var(--muted); font-style:italic; }
  .thresh .tof { color:var(--muted); font-size:12.5px; }
  .thresh code { font-family:'JetBrains Mono',ui-monospace,Consolas,monospace; font-size:12px; color:var(--goldb); font-style:normal; }
  .term { color:var(--muted); font-size:10.5px; font-family:'EB Garamond',serif; }
  .chpos { font-size:14px; color:var(--muted); margin-bottom:14px; }
  .chpos b { color:var(--ivory); }
  .chpos .y { color:var(--green); } .chpos .n { color:var(--red); } .chpos .a { color:var(--muted); }
  .delib.awaiting { opacity:.5; }
  .dvote.pending { color:var(--muted); border-color:rgba(139,147,184,.3); }
  .drat.none { color:var(--muted); font-style:italic; }
  .sigtag { font-family:'Cinzel',serif; font-size:9px; letter-spacing:.08em; text-transform:uppercase; border-radius:999px; padding:1px 8px; margin-left:8px; white-space:nowrap; }
  .sigtag.signed { color:var(--green); border:1px solid rgba(75,189,136,.5); }
  .sigtag.unsigned { color:var(--muted); border:1px solid rgba(139,147,184,.35); }
  .signbox { margin-top:12px; padding:10px 13px; border-radius:9px; border:1px dashed rgba(245,210,122,.4); color:var(--muted); font-size:13px; line-height:1.55; }
  .signbox b { color:var(--goldb); }
  #vote-msg { color:var(--goldb); font-size:13px; margin-top:9px; min-height:1.2em; }
  .delib-list { display:flex; flex-direction:column; gap:14px; }
  .delib { display:grid; grid-template-columns:76px minmax(0,1fr); gap:13px; align-items:start; }
  .delib > * { min-width:0; }
  .dvote { font-family:'Cinzel',serif; font-size:11px; font-weight:700; letter-spacing:.08em; text-transform:uppercase; text-align:center; padding:6px 0; border-radius:8px; border:1px solid; }
  .dvote.y { color:var(--green); border-color:rgba(75,189,136,.5); } .dvote.n { color:var(--red); border-color:rgba(217,105,95,.5); } .dvote.a { color:var(--muted); border-color:rgba(139,147,184,.4); }
  .dname { color:var(--ivory); font-family:'Cinzel',serif; font-size:14px; font-weight:700; letter-spacing:.02em; }
  .drole { color:var(--muted); font-family:'EB Garamond',serif; font-size:12.5px; font-weight:400; letter-spacing:0; text-transform:none; margin-left:6px; }
  .drat { color:var(--body); font-size:14.5px; line-height:1.5; margin-top:3px; }
  .castnote { font-size:13.5px; color:var(--muted); margin:2px 0 12px; line-height:1.5; }
  .fl-note, .dr-note { color:var(--muted); font-size:13.5px; line-height:1.55; margin-bottom:12px; }
  .fl-row { display:flex; gap:10px; flex-wrap:wrap; }
  .fl-form { margin:0; }
  .fl { font-family:'Cinzel',serif; font-size:11.5px; letter-spacing:.06em; text-transform:uppercase; font-weight:700; color:var(--muted); background:transparent; border:1px solid rgba(139,147,184,.35); border-radius:999px; padding:9px 18px; cursor:pointer; }
  .fl:hover { border-color:var(--goldb); color:var(--goldb); }
  .fl-n { font-family:'JetBrains Mono',monospace; font-size:10px; opacity:.8; margin-left:4px; }
  .fl.up.fl-discuss { color:var(--goldb); border-color:rgba(245,210,122,.55); background:rgba(245,210,122,.08); }
  .fl.up.fl-ready { color:var(--green); border-color:rgba(75,189,136,.55); background:rgba(75,189,136,.08); }
  .fl.up.fl-blocked { color:var(--red); border-color:rgba(217,105,95,.55); background:rgba(217,105,95,.08); }
  .fl.mine { box-shadow:inset 0 0 0 1px currentColor; }
  .fl-who { color:var(--muted); font-size:13px; margin-top:10px; }
  .fl-who b { color:var(--body); }
  .draft { width:100%; min-height:120px; background:var(--forum); border:1px solid rgba(201,137,42,.25); border-radius:10px; color:var(--body); font-family:'EB Garamond',Georgia,serif; font-size:15px; line-height:1.55; padding:12px 14px; resize:vertical; }
  .draft:focus { outline:none; border-color:rgba(245,210,122,.6); }
  .dr-status { color:var(--muted); font-size:12px; margin-top:7px; min-height:1.1em; font-style:italic; }
  .dr-use { display:inline-block; margin-top:10px; font-family:'Cinzel',serif; font-size:11.5px; letter-spacing:.06em; text-transform:uppercase; color:var(--goldb); text-decoration:none; border-bottom:1px dotted rgba(201,137,42,.5); }
  .dr-use:hover { color:var(--gold); }
  .castradios { display:flex; gap:10px; margin:6px 0 12px; flex-wrap:wrap; }
  .cr { display:inline-flex; align-items:center; gap:7px; border:1px solid rgba(201,137,42,.3); border-radius:999px; padding:8px 16px; cursor:pointer; font-size:14px; color:var(--body); }
  .cr input { accent-color:var(--gold); }
  .cr:hover { border-color:rgba(245,210,122,.6); }
  .castform textarea { width:100%; min-height:78px; background:var(--forum); border:1px solid rgba(201,137,42,.25); border-radius:10px; color:var(--body); font-family:'EB Garamond',Georgia,serif; font-size:15px; padding:11px 13px; resize:vertical; }
  .cast-btn { margin-top:12px; font-family:'Cinzel',serif; font-size:12px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; color:var(--forum); background:linear-gradient(180deg,var(--goldb),var(--gold)); border:0; border-radius:10px; padding:11px 22px; cursor:pointer; }
  .onward { display:flex; gap:12px; flex-wrap:wrap; margin:6px 0 2px; }
  .submit-btn { display:inline-block; font-family:'Cinzel',serif; font-size:13px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; color:var(--forum); background:linear-gradient(180deg,var(--goldb),var(--gold)); text-decoration:none; border-radius:10px; padding:12px 22px; }
  .submit-btn:hover { filter:brightness(1.05); }
  .rat-btn { display:inline-block; font-family:'Cinzel',serif; font-size:13px; letter-spacing:.08em; text-transform:uppercase; font-weight:700; color:var(--goldb); border:1px solid rgba(245,210,122,.45); text-decoration:none; border-radius:10px; padding:12px 22px; }
  .rat-btn:hover { background:rgba(245,210,122,.1); }
  footer { color:var(--muted); font-size:13px; border-top:1px solid rgba(201,137,42,.15); }
</style>
</head>
<body>
<header>
  <div class="hin">
    <div class="name">CE<b>LL</b>A</div>
    <a class="back" href="/">← All governance actions</a> &nbsp;·&nbsp; <a class="back" href="/constitution">Constitution</a>
  </div>
</header>
<main>
  <div class="type">{{.Type}}</div>
  <h1>{{if .Title}}{{.Title}}{{else}}<span class="muted">(no anchored title)</span>{{end}}</h1>
  {{if .Settled}}
  <div class="settled {{.Status}}">
    <span class="st-badge">{{.Status}}</span>
    <span class="st-note">
      {{if eq .Status "Enacted"}}This action is in force. The chain has already executed it.
      {{else if eq .Status "Ratified"}}This action passed and is awaiting enactment. It is no longer the committee's to decide.
      {{else if eq .Status "Dropped"}}This action was removed without being enacted.
      {{else}}This action expired without reaching a decision.{{end}}
    </span>
  </div>
  {{else if .Deadline.Epoch}}
  <div class="deadline {{.Deadline.Urgency}}">
    <span class="cd">{{if .Deadline.Countdown}}{{.Deadline.Countdown}}{{else}}expires epoch {{.Deadline.Epoch}}{{end}}</span>
    <span class="dlwhen">{{.Deadline.When}}</span>
    {{if and (not .Deadline.Expired) .Deadline.Known}}<span class="dlnote">An action that expires unvoted is an abstention the committee never chose.</span>{{end}}
  </div>
  {{end}}

  {{if and .MetaValid.Valid (not .MetaValid.Bool)}}
  <div class="anchorbad">
    <b>The anchored metadata does not match the chain.</b> The document at the anchor does not hash to what the proposer committed to on-chain, so the title, abstract and motivation below may not be what was actually proposed. Read the payload, not the prose.
  </div>
  {{end}}

  <div class="meta">
    Seen {{date .BlockTime}}{{if .ProposedEpoch.Valid}} · proposed epoch {{.ProposedEpoch.Int64}}{{end}}{{if .Deposit}} · deposit &#8371;{{depositADA .Deposit}}{{end}}<br>
    <code>{{.ProposalID}}</code>
    {{if .ReturnAddress}}<br>Proposer (deposit returns to) <code>{{.ReturnAddress}}</code>{{end}}
    <br><a href="https://adastat.net/governances/{{.GovID}}" target="_blank" rel="noopener">View on AdaStat &#8599;</a>
    {{if .MetaURL}}&nbsp;·&nbsp;<a href="{{.MetaURL}}" rel="noopener">Anchor metadata &#8599;</a>{{end}}
  </div>

  {{template "payload" .}}

  {{if .HasAlignment}}
  <div class="card">
    <h2>Constitutional alignment</h2>
    <div class="al-lead">{{.Alignment.Lead}}</div>
    <div class="al-arts">
      {{range .Alignment.Articles}}<a class="al-art" href="/constitution?focus={{.ID}}#{{.ID}}">{{.Title}} &rarr;</a>{{end}}
    </div>
    <div class="al-note">A starting point for reading, not a boundary on it. The committee judges the action against the whole Constitution &mdash; including the article Cella did not think to name.</div>
  </div>
  {{end}}

  {{if .HasReview}}
  <div class="card">
    <h2>AI-assisted constitutionality review</h2>
    <span class="pill {{.Review.Verdict}}">{{.Review.Verdict}}</span>
    <div class="rsum">{{.Review.Summary}}</div>
    {{if .Review.Model}}<div class="rmodel">Model: {{.Review.Model}} — the committee decides and signs.</div>{{end}}
  </div>
  {{end}}

  {{if .Abstract}}
  <div class="card">
    <h2>Abstract</h2>
    <div class="abstract">{{.AbstractHTML}}</div>
  </div>
  {{end}}

  {{if or .Motivation .ProposerRationale}}
  <div class="card">
    <h2>The proposer's case</h2>
    <div class="pcase-note">Written by the proposer, not by the committee. Read it as an argument, not as a finding.</div>
    {{if .Motivation}}
    <details class="pcase" open>
      <summary>Motivation &mdash; why they say it is needed</summary>
      <div class="abstract">{{.MotivationHTML}}</div>
    </details>
    {{end}}
    {{if .ProposerRationale}}
    <details class="pcase">
      <summary>Their rationale</summary>
      <div class="abstract">{{.ProposerRationaleHTML}}</div>
    </details>
    {{end}}
  </div>
  {{end}}

  {{template "votingcontext" .}}

  {{if .You}}
  <div class="card" id="chamber">
    <h2>Raise a hand</h2>
    <div class="fl-note">A flag is addressed to your co-delegates. Each of you raises your own; nobody can lower yours but you.</div>
    <div class="fl-row">
      {{range .Flags}}
      <form method="post" action="/flag" class="fl-form">
        <input type="hidden" name="slug" value="{{$.Slug}}">
        <input type="hidden" name="csrf" value="{{$.CSRF}}">
        <input type="hidden" name="flag" value="{{.Kind}}">
        <button type="submit" class="fl fl-{{.Kind}} {{if .Mine}}mine{{end}} {{if .Raised}}up{{end}}">
          {{.Label}}{{if .Raised}} <span class="fl-n">{{len .Members}}</span>{{end}}
        </button>
      </form>
      {{end}}
    </div>
    {{range .Flags}}{{if .Raised}}
    <div class="fl-who"><b>{{.Label}}:</b> {{range $i, $m := .Members}}{{if $i}}, {{end}}{{$m}}{{end}}</div>
    {{end}}{{end}}
  </div>

  <div class="card">
    <h2>Your private notes</h2>
    <div class="dr-note">Only you can read these. They are not a position and they are never published &mdash; think out loud here, then record your position below when you are ready.</div>
    <textarea id="draft" class="draft" placeholder="What you make of this action, so far…">{{.Draft}}</textarea>
    <div class="dr-status" id="draft-status"></div>
    <a class="dr-use" href="/rationale/{{.Slug}}">Take this into the committee's rationale &rarr;</a>
  </div>
  {{end}}

  <div class="card">
    <h2>Chamber deliberation — {{.BodyName}}</h2>
    <div class="chpos">Chamber position: <b>{{.ChamberPosition}}</b> &nbsp;·&nbsp; <span class="y">{{.Tally.Yes}} Yes</span> · <span class="n">{{.Tally.No}} No</span> · <span class="a">{{.Tally.Abstain}} Abstain</span> · <span class="a">{{.Tally.DidNotVote}} awaiting</span></div>
    <div class="delib-list">
      {{range .Deliberation}}
      <div class="delib{{if not .Recorded}} awaiting{{end}}">
        {{if .Recorded}}
        <div class="dvote {{if eq .Vote "Yes"}}y{{else if eq .Vote "No"}}n{{else}}a{{end}}">{{.Vote}}</div>
        {{else}}
        <div class="dvote pending">—</div>
        {{end}}
        <div>
          <div class="dname">{{.Member.Name}} <span class="drole">{{.Member.Role}}</span>
            {{if .Recorded}}{{if .Signed}}<span class="sigtag signed" title="Signed by the delegate's wallet">&#10003; signed</span>{{else}}<span class="sigtag unsigned" title="Recorded from a session, not signed by a wallet">unsigned</span>{{end}}{{end}}
          </div>
          {{if .Recorded}}
            {{if .Rationale}}<div class="drat">{{.Rationale}}</div>{{else}}<div class="drat none">Recorded without a rationale.</div>{{end}}
          {{else}}
            <div class="drat none">Has not recorded a position yet.</div>
          {{end}}
        </div>
      </div>
      {{end}}
    </div>
  </div>

  {{if .You}}
  <div class="card" id="your-position">
    <h2>Your position &middot; {{.You}}</h2>
    {{if .YouRecorded}}<div class="castnote">You have recorded this position. Update it any time before the committee submits.</div>{{else}}<div class="castnote">You have not recorded a position on this action yet. Your co-delegates see it as soon as you do.</div>{{end}}
    <form method="post" action="/vote" class="castform" id="castform">
      <input type="hidden" name="slug" value="{{.Slug}}">
      <input type="hidden" name="csrf" value="{{.CSRF}}">
      <input type="hidden" name="signature" id="vote-sig">
      <input type="hidden" name="key" id="vote-key">
      <div class="castradios">
        <label class="cr"><input type="radio" name="vote" value="Yes" {{if eq .YourVote "Yes"}}checked{{end}}>Yes</label>
        <label class="cr"><input type="radio" name="vote" value="No" {{if eq .YourVote "No"}}checked{{end}}>No</label>
        <label class="cr"><input type="radio" name="vote" value="Abstain" {{if eq .YourVote "Abstain"}}checked{{end}}>Abstain</label>
      </div>
      <textarea name="rationale" id="vote-rationale" placeholder="Your rationale (recorded for the body)…">{{.YourRationale}}</textarea>
      <div>
        <button type="submit" class="cast-btn" id="cast-btn">
          {{if .YouCanSign}}Sign &amp; {{if .YouRecorded}}update{{else}}record{{end}} my position{{else}}{{if .YouRecorded}}Update my position{{else}}Record my position{{end}}{{end}}
        </button>
      </div>
      <div id="vote-msg"></div>
      {{if .YouCanSign}}
      <div class="signbox">Your wallet will show you the position you are about to record and ask you to sign it. <b>No funds move.</b> The signature is what makes this position provably yours rather than merely whatever your session said — and the body's split is published in the committee's anchored rationale.</div>
      {{else}}
      <div class="signbox">No wallet is registered against your name in the roster, so this position will be recorded <b>unsigned</b> — attributable only to your session. Register a wallet address to sign your positions.</div>
      {{end}}
    </form>
  </div>
  {{end}}

  <div class="onward">
    <a class="rat-btn" href="/rationale/{{.Slug}}">Author the committee rationale &#8594;</a>
    <a class="submit-btn" href="/submit/{{.Slug}}">Submit committee vote on-chain &#8594;</a>
  </div>

  <div class="card">
    <h2>Constitutional Committee {{if .Seats}}&mdash; {{.Seats}} authorized seats{{end}}</h2>
    {{if .ThresholdKnown}}
    <div class="thresh {{if .ThresholdMet}}met{{end}}">
      {{if .ThresholdMet}}
        <b>&#10003; Threshold met</b> &mdash; {{.Yes}} of the {{.YesNeeded}} Yes votes needed to ratify.
      {{else}}
        <b>{{.Yes}} of {{.YesNeeded}}</b> Yes votes needed to ratify
        <span class="tof">({{.QuorumFraction}} of {{.Seats}} authorized seats, rounded up)</span>
      {{end}}
    </div>
    {{else}}
    <div class="thresh unknown">The committee's threshold is unknown &mdash; run <code>cella ingest</code> to read it from the chain rather than guessing it.</div>
    {{end}}
    <div class="tally"><b class="y">{{.Yes}} Yes</b> · <b class="n">{{.No}} No</b> · <b class="a">{{.Abstain}} Abstain</b> <span class="seatsof">of {{len .Committee}} seats</span></div>
    <table class="votes">
      <thead><tr><th>Vote</th><th>Committee member</th><th>Rationale</th></tr></thead>
      <tbody>
        {{range .Committee}}
        <tr class="{{if not .Voted}}pending{{end}}">
          <td class="vote">{{if .Voted}}<b class="{{if eq .Vote "Yes"}}y{{else if eq .Vote "No"}}n{{else}}a{{end}}">{{.Vote}}</b>{{else}}<span class="await">Awaiting</span>{{end}}</td>
          <td class="cc">{{if .Name}}<span class="ccname">{{.Name}}</span><br>{{end}}{{.Credential}}{{if .TermEnds}}<br><span class="term">term ends epoch {{.TermEnds}}</span>{{end}}</td>
          <td>{{if .Rationale}}<a href="{{.Rationale}}" rel="noopener">rationale ↗</a>{{else}}<span class="muted">—</span>{{end}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </div>
</main>
<footer><div class="fin">Cella · built &amp; maintained by Awen LLC · Apache-2.0</div></footer>
{{if .You}}
<script>
// Autosave the private notes. A delegate who has to remember to press save is a
// delegate who will lose a paragraph of thinking to a closed tab.
(function () {
  var box = document.getElementById('draft'), status = document.getElementById('draft-status');
  if (!box) return;
  var timer = null, last = box.value;

  function save() {
    if (box.value === last) return;
    var body = box.value;
    status.textContent = 'Saving…';
    fetch('/draft', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ slug: {{.Slug}}, csrf: {{.CSRF}}, body: body })
    }).then(function (r) {
      if (!r.ok) throw new Error(r.status);
      last = body;
      status.textContent = 'Saved ' + new Date().toLocaleTimeString();
    }).catch(function () {
      status.textContent = 'Could not save — your notes are still in the box.';
    });
  }

  box.addEventListener('input', function () { clearTimeout(timer); timer = setTimeout(save, 800); });
  // Do not lose the last few keystrokes to a closed tab.
  window.addEventListener('beforeunload', save);
  box.addEventListener('blur', save);
})();
</script>
{{end}}
{{if .YouCanSign}}
<script>
// Sign the position before recording it. The message is fetched from the server
// rather than composed here: the server decides what a signature means, and it
// re-derives the same bytes when it verifies. Anything this script invented
// would simply fail to verify.
(function () {
  var form = document.getElementById('castform');
  var btn  = document.getElementById('cast-btn');
  var msg  = document.getElementById('vote-msg');
  var sigF = document.getElementById('vote-sig');
  var keyF = document.getElementById('vote-key');
  var signed = false;

  function chosen() {
    var r = form.querySelector('input[name=vote]:checked');
    return r ? r.value : '';
  }
  function errText(e) {
    if (!e) return 'unknown';
    if (typeof e === 'string') return e;
    return e.info || e.message || String(e);
  }
  function pickWallet() {
    var keys = Object.keys(window.cardano || {}).filter(function (k) {
      var w = window.cardano[k];
      return w && typeof w.enable === 'function' && typeof w.icon !== 'undefined';
    });
    return keys.length ? keys[0] : null;
  }

  form.addEventListener('submit', async function (e) {
    if (signed) return;            // already signed — let it through
    e.preventDefault();

    if (!chosen()) { msg.textContent = 'Choose Yes, No or Abstain first.'; return; }

    var key = pickWallet();
    if (!key) { msg.textContent = 'No Cardano wallet found in this browser. Install Eternl or Lace to sign your position.'; return; }

    btn.disabled = true;
    try {
      // Ask the server what, exactly, is being signed.
      msg.textContent = 'Preparing the position…';
      var body = new URLSearchParams({
        slug: form.slug.value,
        csrf: form.csrf.value,
        vote: chosen(),
        rationale: document.getElementById('vote-rationale').value
      });
      var prep = await fetch('/vote/prepare', {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body
      });
      if (!prep.ok) {
        var pe = await prep.json().catch(function () { return {}; });
        msg.textContent = 'Could not prepare the position (' + (pe.error || prep.status) + ').';
        btn.disabled = false;
        return;
      }
      var prepared = await prep.json();

      var w = window.cardano[key];
      msg.textContent = 'Approve the signature in ' + (w.name || key) + '…';
      var api = await w.enable();
      var rew = await api.getRewardAddresses();
      var addr = (rew && rew[0]) || (await api.getUsedAddresses())[0];
      if (!addr) { msg.textContent = 'Could not read an address from your wallet.'; btn.disabled = false; return; }

      var out = await api.signData(addr, prepared.hex);
      sigF.value = out.signature;
      keyF.value = out.key;

      signed = true;
      msg.textContent = 'Signed. Recording…';
      form.submit();
    } catch (err) {
      msg.textContent = 'Signing cancelled or failed (' + errText(err) + '). Your position was not recorded.';
      btn.disabled = false;
    }
  });
})();
</script>
{{end}}
</body>
</html>`
