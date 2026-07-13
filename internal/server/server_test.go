package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Awen-online/cella/internal/koios"
	"github.com/Awen-online/cella/internal/store"
)

// seedServer builds a Server backed by a throwaway SQLite database seeded with
// one titled governance action, two Constitutional Committee votes (plus a DRep
// vote that must be filtered out), and an AI review.
func seedServer(t *testing.T) (*Server, koios.GovernanceAction) {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	act := koios.GovernanceAction{
		ProposalID: "gov_action1abc#0",
		TxHash:     "abc123def456",
		Index:      0,
		Type:       "TreasuryWithdrawals",
		BlockTime:  1_700_000_000,
		MetaJSON:   json.RawMessage(`{"body":{"title":"Fund a public good","abstract":"A treasury withdrawal to fund an open-source library."}}`),
	}
	if _, err := db.UpsertActions([]koios.GovernanceAction{act}); err != nil {
		t.Fatalf("seed actions: %v", err)
	}

	votes := []koios.Vote{
		{VoterRole: "ConstitutionalCommittee", VoterID: "cc_hot_1", Vote: "Yes", MetaURL: "https://example.org/rationale1", BlockTime: 1_700_000_100},
		{VoterRole: "ConstitutionalCommittee", VoterID: "cc_hot_2", Vote: "No", BlockTime: 1_700_000_200},
		{VoterRole: "DRep", VoterID: "drep_1", Vote: "Yes", MetaURL: "ipfs://drep-rationale-cid", BlockTime: 1_700_000_300},
	}
	if _, err := db.UpsertVotes(act.ProposalID, votes); err != nil {
		t.Fatalf("seed votes: %v", err)
	}
	if err := db.UpsertReview(act.ProposalID, "constitutional", "Aligns with treasury guardrails.", "test-model"); err != nil {
		t.Fatalf("seed review: %v", err)
	}

	// Demo mode on: most tests exercise the chamber via roster sign-in. The
	// tests that care about the gate build their own server.
	//
	// The body is a fixture, deliberately not the roster Cella ships with: a
	// test that breaks because a real member joined or left the Curia is a test
	// asserting the wrong thing.
	s := New(db, Options{Secret: "test-secret", Demo: true, Body: testBody})
	return s, act
}

// testBody is the five-delegate consortium the chamber tests deliberate as.
var testBody = Body{
	Name:  "Test Consortium",
	Short: "The Test",
	Kind:  "Constitutional Committee member",
	Blurb: "A fixture body.",
	Members: []Member{
		{Name: "Faustina Vela", Role: "Delegate · Treasury"},
		{Name: "Cassius Aurel", Role: "Delegate · Parameters"},
		{Name: "Junia Marcia", Role: "Delegate · Precedent"},
		{Name: "Titus Varo", Role: "Delegate · Outreach"},
		{Name: "Cullah", Role: "Delegate · At-large"},
	},
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandleIndex(t *testing.T) {
	s, act := seedServer(t)
	rec := get(t, s, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	slug := fmt.Sprintf("%s-%d", act.TxHash, act.Index)
	for _, want := range []string{"Fund a public good", "/action/" + slug, "TreasuryWithdrawals", "/constitution"} {
		if !strings.Contains(body, want) {
			t.Errorf("index page missing %q", want)
		}
	}
}

func TestHandleDetail(t *testing.T) {
	s, act := seedServer(t)
	slug := fmt.Sprintf("%s-%d", act.TxHash, act.Index)
	rec := get(t, s, "/action/"+slug)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Fund a public good",               // title
		"A treasury withdrawal",            // abstract
		"cc_hot_1",                         // CC voter (full id on detail)
		"rationale",                        // rationale link
		"constitutional",                   // AI verdict
		"Aligns with treasury guardrails.", // AI summary
		"1 Yes", "1 No",                    // tally (CC only — the DRep is not counted in it)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail page missing %q", want)
		}
	}

	// The DRep's vote is context, never part of the committee's tally. It must
	// not be mistaken for a seat: the count above stays 1 Yes / 1 No, and the
	// DRep must not appear in the committee's own table of voters.
	cc, _, _ := strings.Cut(body, "How the rest of the chain is voting")
	if strings.Contains(cc, "drep_1") {
		t.Error("a DRep appears in the committee's own vote table; it is not a committee member")
	}
}

// The committee is not bound by how the delegate representatives voted, but it
// should be able to read why they voted that way. A DRep who published a
// rationale must be reachable from the action page in one click.
func TestDetailLinksDRepRationales(t *testing.T) {
	s, act := seedServer(t)

	// The panel only renders when the chain has reported a tally to sit above it.
	if err := s.db.SaveVotingSummary(act.ProposalID, koios.VotingSummary{DRepYesVotes: 1, DRepYesPct: 100}); err != nil {
		t.Fatalf("seed voting summary: %v", err)
	}

	body := get(t, s, "/action/"+fmt.Sprintf("%s-%d", act.TxHash, act.Index)).Body.String()

	// ipfs:// is rewritten to a gateway — a browser cannot follow it otherwise.
	if !strings.Contains(body, "https://ipfs.io/ipfs/drep-rationale-cid") {
		t.Error("the DRep's published rationale is not linked")
	}
	if !strings.Contains(body, "adastat.net/dreps/drep_1") {
		t.Error("the DRep is not linked to a block explorer")
	}
	if !strings.Contains(body, "1 published a rationale") {
		t.Error("the count of published rationales is not shown")
	}
}

// A rationale anchor is written by a stranger and read off the chain. Cella will
// render it as a link, so anything that is not a web or IPFS address must be
// dropped rather than trusted into an href.
func TestRationaleLinkRejectsHostileSchemes(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://example.org/why", "https://example.org/why"},
		{"http://example.org/why", "http://example.org/why"},
		{"ipfs://QmAbc123", "https://ipfs.io/ipfs/QmAbc123"},
		{"ipfs://QmAbc123/doc.json", "https://ipfs.io/ipfs/QmAbc123/doc.json"},
		{"javascript:alert(1)", ""},
		{"data:text/html;base64,PHNjcmlwdD4=", ""},
		{"file:///etc/passwd", ""},
		{"", ""},
		{"   ", ""},
	} {
		if got := rationaleLink(tc.in); got != tc.want {
			t.Errorf("rationaleLink(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHandleDetailNotFound(t *testing.T) {
	s, _ := seedServer(t)
	if rec := get(t, s, "/action/nonexistent-9"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown action status = %d, want 404", rec.Code)
	}
}

func TestHandleConstitution(t *testing.T) {
	s, _ := seedServer(t)

	rec := get(t, s, "/constitution")
	if rec.Code != http.StatusOK {
		t.Fatalf("constitution status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"PREAMBLE",  // the text itself
		"Revision",  // the version switcher
		"v2.4",      // the current revision
		"Contents",  // the table of contents
		`id="q"`,    // the in-page search
		"permalink", // an anchor on every article
	} {
		if !strings.Contains(body, want) {
			t.Errorf("constitution page missing %q", want)
		}
	}

	// The articles the action page deep-links into must be anchored here.
	if !strings.Contains(body, `id="article-iii-constitutional-committee"`) {
		t.Error("Article III has no anchor; the action page's alignment links would be dead")
	}

	// An older revision renders too, and an unknown revision falls back to current.
	if rec := get(t, s, "/constitution?v=v0"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "PREAMBLE") {
		t.Errorf("v0 constitution did not render (code=%d)", rec.Code)
	}
	if rec := get(t, s, "/constitution?v=bogus"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "PREAMBLE") {
		t.Errorf("unknown revision did not fall back to current (code=%d)", rec.Code)
	}
}

// A governance action points the committee at the articles that govern it. The
// mapping is only useful if the anchors it names actually exist, and the whole
// point is that the Constitution stops being a wall of text nobody cites.
func TestAlignmentLinksIntoTheConstitution(t *testing.T) {
	cases := map[string]string{
		"TreasuryWithdrawals": "appendix-i-cardano-blockchain-guardrails",
		"NewCommittee":        "article-iii-constitutional-committee",
		"NoConfidence":        "article-iii-constitutional-committee",
		"NewConstitution":     "article-iv-amendment-process",
		"HardForkInitiation":  "article-i-cardano-blockchain-tenets-and-guardrails",
		"InfoAction":          "article-ii-community-and-governance",
	}
	for typ, wantAnchor := range cases {
		t.Run(typ, func(t *testing.T) {
			a, ok := alignmentFor(typ)
			if !ok {
				t.Fatalf("no constitutional alignment for %s", typ)
			}
			if a.Lead == "" {
				t.Error("the alignment says nothing about why these articles apply")
			}
			var found bool
			for _, art := range a.Articles {
				if art.ID == wantAnchor {
					found = true
				}
			}
			if !found {
				t.Errorf("%s does not point at #%s; got %v", typ, wantAnchor, a.Articles)
			}
		})
	}

	// An action type Cella has never seen must say nothing, rather than point the
	// committee at an article that may have no bearing on it.
	if _, ok := alignmentFor("SomeFutureAction"); ok {
		t.Error("an unknown action type was given a constitutional alignment")
	}
}

func TestHealthz(t *testing.T) {
	s, _ := seedServer(t)
	rec := get(t, s, "/healthz")
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("healthz = %d %q, want 200 \"ok\"", rec.Code, rec.Body.String())
	}
}
