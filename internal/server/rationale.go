package server

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/Awen-online/cella/internal/constitution"
	"github.com/Awen-online/cella/internal/rationale"
	"github.com/Awen-online/cella/internal/store"
)

// The committee's rationale is the document the chamber's deliberation
// ultimately produces: a citable statement of why the committee voted as it
// did, anchored on-chain alongside the vote as a CIP-136 JSON-LD file.
//
// Two routes serve it:
//
//	/rationale/{slug}         author and review the rationale
//	/rationale/{slug}.jsonld  download the exact bytes that get anchored
//
// The .jsonld bytes are not a preview. They are the artifact: their
// blake2b-256 is the anchor hash submitted with the vote, and the same value
// `cardano-cli hash anchor-data --file-text` prints for the downloaded file.

// tally is how the body's delegates split on an action. It is the source of
// the CIP-136 internalVote block, which is how a multi-member committee shows
// the chain that its single vote came from a real internal split.
type tally struct {
	Yes, No, Abstain, DidNotVote int
}

// Decision is the committee's resolved position: the plurality of the
// delegates who actually voted. With no votes recorded, or a tie, the
// committee abstains rather than guessing.
func (t tally) Decision() string {
	switch {
	case t.Yes > t.No && t.Yes >= t.Abstain:
		return "Yes"
	case t.No > t.Yes && t.No >= t.Abstain:
		return "No"
	default:
		return "Abstain"
	}
}

// Recorded is how many delegates have taken a position.
func (t tally) Recorded() int { return t.Yes + t.No + t.Abstain }

// Seats is how many delegates the body has — the denominator for quorum.
func (t tally) Seats() int { return t.Recorded() + t.DidNotVote }

// tallyFrom counts the body's recorded positions from an already-fetched vote
// map. Roster members who have not recorded one are counted as didNotVote — the
// CIP distinguishes abstaining (a deliberate position) from simply not voting.
func tallyFrom(votes map[string]store.MemberVote, body Body) (tally, []string) {
	var t tally
	var authors []string
	for _, m := range body.Members {
		v, ok := votes[m.Name]
		if !ok || v.Vote == "" {
			t.DidNotVote++
			continue
		}
		// A delegate who took a position stands behind the document.
		authors = append(authors, m.Name)
		switch v.Vote {
		case "Yes":
			t.Yes++
		case "No":
			t.No++
		default:
			t.Abstain++
		}
	}
	return t, authors
}

// tallyFor counts the body's recorded positions on a single action.
func (s *Server) tallyFor(proposalID string) (tally, []string, error) {
	votes, err := s.db.MemberVotesFor(proposalID)
	if err != nil {
		return tally{}, nil, err
	}
	t, authors := tallyFrom(votes, s.body)
	return t, authors, nil
}

// docFor assembles the CIP-136 document for an action from the stored
// rationale and the chamber's internal split.
func (s *Server) docFor(a store.ActionRow) (rationale.Doc, store.Rationale, error) {
	r, _, err := s.db.RationaleFor(a.ProposalID)
	if err != nil {
		return rationale.Doc{}, r, err
	}
	t, authors, err := s.tallyFor(a.ProposalID)
	if err != nil {
		return rationale.Doc{}, r, err
	}

	body := rationale.Body{
		Summary:                   r.Summary,
		RationaleStatement:        r.Statement,
		PrecedentDiscussion:       r.Precedent,
		CounterargumentDiscussion: r.Counterargument,
		Conclusion:                r.Conclusion,
	}
	if t.Recorded() > 0 || t.DidNotVote > 0 {
		body.InternalVote = &rationale.InternalVote{
			Constitutional:   t.Yes,
			Unconstitutional: t.No,
			Abstain:          t.Abstain,
			DidNotVote:       t.DidNotVote,
		}
	}
	// Cite what the committee actually judged against, and the action itself, so
	// the rationale stays verifiable years from now — when "the Constitution"
	// will mean a later revision than the one in force today.
	_, ver, err := constitution.Text("")
	if err != nil {
		return rationale.Doc{}, r, err
	}
	body.References = []rationale.Reference{
		{
			Type:  "RelevantArticles",
			Label: "Cardano Constitution " + ver.Label + " — the revision this committee judged against",
			URI:   "https://github.com/IntersectMBO/cardano-constitution",
		},
		{
			Type:  "Other",
			Label: "The governance action on a block explorer",
			URI:   s.net.ExplorerAction(a.GovID()),
		},
	}

	return rationale.New(body, authors), r, nil
}

// rationaleView drives the authoring page.
type rationaleView struct {
	store.ActionRow
	store.Rationale

	Tally      tally
	Decision   string
	Authors    []string
	Authored   bool
	Valid      bool
	Problem    string
	AnchorHash string
	JSONLD     string
	Preview    template.HTML
	SummaryMax int
	You        string
	CSRF       string
}

// handleRationale serves the rationale authoring page, the save, and the
// .jsonld download.
func (s *Server) handleRationale(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/rationale/")
	download := strings.HasSuffix(slug, ".jsonld")
	slug = strings.TrimSuffix(slug, ".jsonld")
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

	if r.Method == http.MethodPost {
		s.saveRationale(w, r, a)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	doc, stored, err := s.docFor(a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonld, err := doc.JSONLD()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if download {
		// Refuse to hand out a document the CIP would reject: an anchor hash
		// over an invalid rationale is worse than no file at all, because it
		// looks submittable.
		if err := doc.Validate(); err != nil {
			http.Error(w, "this rationale is not ready to anchor: "+err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/ld+json")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=%q", "rationale-"+a.Slug()+".jsonld"))
		w.Write(jsonld)
		return
	}

	t, authors, err := s.tallyFor(a.ProposalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sessionID, _ := s.member(r)

	v := rationaleView{
		ActionRow:  a,
		Rationale:  stored,
		Tally:      t,
		Decision:   t.Decision(),
		Authors:    authors,
		Authored:   !stored.Empty(),
		AnchorHash: rationale.AnchorHash(jsonld),
		JSONLD:     string(jsonld),
		Preview:    mdHTML(stored.Statement),
		SummaryMax: rationale.SummaryLimit,
		You:        strings.TrimSuffix(sessionID, " (demo)"),
		CSRF:       s.csrfToken(sessionID),
	}
	if err := doc.Validate(); err != nil {
		v.Problem = err.Error()
	} else {
		v.Valid = true
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.rtpl.Execute(w, v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// saveRationale records the committee's authored rationale.
func (s *Server) saveRationale(w http.ResponseWriter, r *http.Request, a store.ActionRow) {
	sessionID, ok := s.member(r)
	if !ok {
		http.Redirect(w, r, "/enter", http.StatusFound)
		return
	}
	if !s.checkCSRF(r, sessionID) {
		http.Error(w, "bad or missing CSRF token", http.StatusForbidden)
		return
	}

	rec := store.Rationale{
		Summary:         strings.TrimSpace(r.FormValue("summary")),
		Statement:       strings.TrimSpace(r.FormValue("statement")),
		Precedent:       strings.TrimSpace(r.FormValue("precedent")),
		Counterargument: strings.TrimSpace(r.FormValue("counterargument")),
		Conclusion:      strings.TrimSpace(r.FormValue("conclusion")),
		AuthoredBy:      strings.TrimSuffix(sessionID, " (demo)"),
	}
	if err := s.db.UpsertRationale(a.ProposalID, rec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/rationale/"+a.Slug(), http.StatusFound)
}
