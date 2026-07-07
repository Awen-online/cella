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
