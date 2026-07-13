package store

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Awen-online/cella/internal/koios"
)

func TestUpsertAndActions(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	exp := int64(700)
	actions := []koios.GovernanceAction{
		{
			ProposalID: "gov_action1", Type: "InfoAction", BlockTime: 100, Expiration: &exp,
			MetaJSON: json.RawMessage(`{"body":{"title":"First action"}}`),
		},
		{ProposalID: "gov_action2", Type: "TreasuryWithdrawals", BlockTime: 200},
	}

	n, err := db.UpsertActions(actions)
	if err != nil {
		t.Fatalf("UpsertActions: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d, want 2", n)
	}

	rows, err := db.Actions(10)
	if err != nil {
		t.Fatalf("Actions: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// newest first (block_time desc): gov_action2 (200) before gov_action1 (100).
	if rows[0].ProposalID != "gov_action2" {
		t.Errorf("ordering: first row = %s, want gov_action2", rows[0].ProposalID)
	}
	if rows[1].Title != "First action" {
		t.Errorf("CIP-108 title not stored: got %q", rows[1].Title)
	}

	// Upsert is idempotent on proposal_id and updates fields (no duplicate rows).
	actions[0].Type = "NoConfidence"
	if _, err := db.UpsertActions(actions); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	rows, err = db.Actions(10)
	if err != nil {
		t.Fatalf("Actions after re-upsert: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("after re-upsert got %d rows, want 2 (no duplicates)", len(rows))
	}
	for _, r := range rows {
		if r.ProposalID == "gov_action1" && r.Type != "NoConfidence" {
			t.Errorf("upsert did not update type: got %q, want NoConfidence", r.Type)
		}
	}
}

func TestUpsertVotesFiltersCCAndGroups(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	votes := []koios.Vote{
		{VoterRole: "ConstitutionalCommittee", VoterID: "cc_hot_1", Vote: "Yes", MetaURL: "ipfs://r1", BlockTime: 10},
		{VoterRole: "ConstitutionalCommittee", VoterID: "cc_hot_2", Vote: "No", BlockTime: 11},
		{VoterRole: "DRep", VoterID: "drep_1", Vote: "Yes", BlockTime: 12},
		{VoterRole: "SPO", VoterID: "pool_1", Vote: "Abstain", BlockTime: 13},
	}
	n, err := db.UpsertVotes("gov_action1", votes)
	if err != nil {
		t.Fatalf("UpsertVotes: %v", err)
	}
	if n != 2 {
		t.Fatalf("stored %d votes, want 2 (CC only, DRep/SPO filtered out)", n)
	}

	grouped, err := db.VotesFor([]string{"gov_action1", "gov_action_absent"})
	if err != nil {
		t.Fatalf("VotesFor: %v", err)
	}
	got := grouped["gov_action1"]
	if len(got) != 2 {
		t.Fatalf("VotesFor returned %d votes, want 2", len(got))
	}
	// Ordered by cc_hot_id: cc_hot_1 (with rationale) then cc_hot_2.
	if got[0].VoterID != "cc_hot_1" || got[0].RationaleURL != "ipfs://r1" {
		t.Errorf("first vote = %+v, want cc_hot_1 with rationale ipfs://r1", got[0])
	}
	if got[1].Vote != "No" {
		t.Errorf("second vote = %q, want No", got[1].Vote)
	}

	// Re-upsert is idempotent (same proposal + cc_hot_id), and updates the vote.
	votes[0].Vote = "Abstain"
	if _, err := db.UpsertVotes("gov_action1", votes); err != nil {
		t.Fatalf("re-upsert votes: %v", err)
	}
	grouped, _ = db.VotesFor([]string{"gov_action1"})
	if len(grouped["gov_action1"]) != 2 {
		t.Errorf("after re-upsert got %d votes, want 2 (no duplicates)", len(grouped["gov_action1"]))
	}
}

func TestUpsertReviewRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.UpsertReview("gov_action1", "uncertain", "Not enough detail.", "llama3.1"); err != nil {
		t.Fatalf("UpsertReview: %v", err)
	}
	// Overwrite with an updated verdict (idempotent on proposal_id).
	if err := db.UpsertReview("gov_action1", "constitutional", "Aligns with the treasury rules.", "gpt-4o-mini"); err != nil {
		t.Fatalf("UpsertReview overwrite: %v", err)
	}

	got, err := db.ReviewsFor([]string{"gov_action1", "absent"})
	if err != nil {
		t.Fatalf("ReviewsFor: %v", err)
	}
	r, ok := got["gov_action1"]
	if !ok {
		t.Fatal("review not found")
	}
	if r.Verdict != "constitutional" || r.Model != "gpt-4o-mini" {
		t.Errorf("got %+v, want constitutional / gpt-4o-mini", r)
	}
	if _, ok := got["absent"]; ok {
		t.Error("absent proposal should have no review")
	}
}

func TestRationaleRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, ok, err := db.RationaleFor("gov_action1"); err != nil || ok {
		t.Fatalf("RationaleFor(unauthored) = ok:%v err:%v; want ok:false err:nil", ok, err)
	}

	want := Rationale{
		Summary:         "The committee finds the withdrawal constitutional.",
		Statement:       "The request is proportionate and within the treasury guardrails.",
		Precedent:       "Consistent with the Ouroboros Leios withdrawal.",
		Counterargument: "The amount is large relative to prior awards.",
		Conclusion:      "Approved.",
		AuthoredBy:      "Junia Marcia",
	}
	if err := db.UpsertRationale("gov_action1", want); err != nil {
		t.Fatalf("UpsertRationale: %v", err)
	}

	got, ok, err := db.RationaleFor("gov_action1")
	if err != nil || !ok {
		t.Fatalf("RationaleFor = ok:%v err:%v; want ok:true", ok, err)
	}
	if got.Summary != want.Summary || got.Statement != want.Statement ||
		got.Precedent != want.Precedent || got.Counterargument != want.Counterargument ||
		got.Conclusion != want.Conclusion || got.AuthoredBy != want.AuthoredBy {
		t.Errorf("rationale did not round-trip:\n got %+v\nwant %+v", got, want)
	}
	if got.UpdatedAt == 0 {
		t.Error("UpdatedAt was not stamped")
	}
	if got.Empty() {
		t.Error("an authored rationale reports itself Empty()")
	}

	// A second author revising it replaces the text rather than adding a row.
	revised := want
	revised.Summary = "On reflection, unconstitutional."
	revised.AuthoredBy = "Cullah"
	if err := db.UpsertRationale("gov_action1", revised); err != nil {
		t.Fatalf("UpsertRationale (revision): %v", err)
	}
	got, _, err = db.RationaleFor("gov_action1")
	if err != nil {
		t.Fatalf("RationaleFor: %v", err)
	}
	if got.Summary != revised.Summary || got.AuthoredBy != "Cullah" {
		t.Errorf("revision did not replace the rationale: %+v", got)
	}

	// Rationales are per-action, not global.
	if _, ok, _ := db.RationaleFor("gov_action2"); ok {
		t.Error("a rationale authored for one action leaked to another")
	}
}

func TestRationaleEmpty(t *testing.T) {
	cases := map[string]struct {
		r    Rationale
		want bool
	}{
		"nothing authored":                       {Rationale{}, true},
		"whitespace only":                        {Rationale{Summary: "  ", Statement: "\n\t"}, true},
		"summary only":                           {Rationale{Summary: "s"}, false},
		"statement only":                         {Rationale{Statement: "s"}, false},
		"conclusion but no summary or statement": {Rationale{Conclusion: "Approved."}, true},
	}
	for name, tc := range cases {
		if got := tc.r.Empty(); got != tc.want {
			t.Errorf("%s: Empty() = %v, want %v", name, got, tc.want)
		}
	}
}
