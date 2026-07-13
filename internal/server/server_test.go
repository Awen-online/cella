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
		{VoterRole: "DRep", VoterID: "drep_1", Vote: "Yes", BlockTime: 1_700_000_300}, // must be filtered
	}
	if _, err := db.UpsertVotes(act.ProposalID, votes); err != nil {
		t.Fatalf("seed votes: %v", err)
	}
	if err := db.UpsertReview(act.ProposalID, "constitutional", "Aligns with treasury guardrails.", "test-model"); err != nil {
		t.Fatalf("seed review: %v", err)
	}

	return New(db, "test-secret"), act
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
		"1 Yes", "1 No",                    // tally (CC only)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
	if strings.Contains(body, "drep_1") {
		t.Error("detail page leaked a non-CC (DRep) voter")
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
	for _, want := range []string{"PREAMBLE", "Constitution revision", "v2.4"} {
		if !strings.Contains(body, want) {
			t.Errorf("constitution page missing %q", want)
		}
	}

	// An older revision renders too, and an unknown revision falls back to current.
	if rec := get(t, s, "/constitution?v=v0"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "PREAMBLE") {
		t.Errorf("v0 constitution did not render (code=%d)", rec.Code)
	}
	if rec := get(t, s, "/constitution?v=bogus"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "PREAMBLE") {
		t.Errorf("unknown revision did not fall back to current (code=%d)", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	s, _ := seedServer(t)
	rec := get(t, s, "/healthz")
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("healthz = %d %q, want 200 \"ok\"", rec.Code, rec.Body.String())
	}
}
