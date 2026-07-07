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
