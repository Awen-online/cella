// Package store is Cella's local persistence layer, backed by SQLite via a
// pure-Go driver (modernc.org/sqlite). No CGO is required, so `go build`
// produces a single self-contained binary with no external database server.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Awen-online/cella/internal/koios"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS governance_actions (
  proposal_id TEXT PRIMARY KEY,
  tx_hash     TEXT,
  idx         INTEGER,
  type        TEXT,
  title       TEXT,
  abstract    TEXT,
  meta_url    TEXT,
  block_time  INTEGER,
  expiration  INTEGER,
  raw         TEXT,
  ingested_at INTEGER
);

CREATE TABLE IF NOT EXISTS cc_votes (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  proposal_id   TEXT NOT NULL,
  cc_hot_id     TEXT,
  vote          TEXT,            -- Yes | No | Abstain
  rationale_url TEXT,
  block_time    INTEGER,
  UNIQUE(proposal_id, cc_hot_id)
);

CREATE TABLE IF NOT EXISTS reviews (
  proposal_id TEXT PRIMARY KEY,
  verdict     TEXT,             -- constitutional | unconstitutional | uncertain
  summary     TEXT,
  model       TEXT,
  reviewed_at INTEGER
);
`

// DB wraps the SQLite connection.
type DB struct{ sql *sql.DB }

// Open opens (creating if needed) the SQLite database at path and ensures the
// schema is present.
func Open(path string) (*DB, error) {
	sdb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := sdb.Exec(schema); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	// Idempotent upgrade for databases created before the abstract column;
	// a duplicate-column error just means it is already present.
	if _, err := sdb.Exec(`ALTER TABLE governance_actions ADD COLUMN abstract TEXT`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		// Non-fatal: a fresh database already has the column.
		_ = err
	}
	return &DB{sql: sdb}, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

// UpsertActions inserts or updates governance actions, returning the number
// written.
func (d *DB) UpsertActions(actions []koios.GovernanceAction) (int, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
INSERT INTO governance_actions
  (proposal_id, tx_hash, idx, type, title, abstract, meta_url, block_time, expiration, raw, ingested_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(proposal_id) DO UPDATE SET
  type=excluded.type, title=excluded.title, abstract=excluded.abstract,
  meta_url=excluded.meta_url, block_time=excluded.block_time,
  expiration=excluded.expiration, raw=excluded.raw, ingested_at=excluded.ingested_at`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	n := 0
	for _, a := range actions {
		raw, _ := json.Marshal(a)
		var exp any
		if a.Expiration != nil {
			exp = *a.Expiration
		}
		if _, err := stmt.Exec(a.ProposalID, a.TxHash, a.Index, a.Type, a.Title(), a.Abstract(), a.MetaURL, a.BlockTime, exp, string(raw), now); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

// ActionRow is a governance action as stored, for display.
type ActionRow struct {
	ProposalID string
	Type       string
	Title      string
	Abstract   string
	MetaURL    string
	BlockTime  int64
	Expiration sql.NullInt64
}

// Actions returns stored governance actions, newest first.
func (d *DB) Actions(limit int) ([]ActionRow, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.sql.Query(`
SELECT proposal_id, type, title, COALESCE(abstract, ''), meta_url, block_time, expiration
FROM governance_actions
ORDER BY block_time DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActionRow
	for rows.Next() {
		var r ActionRow
		if err := rows.Scan(&r.ProposalID, &r.Type, &r.Title, &r.Abstract, &r.MetaURL, &r.BlockTime, &r.Expiration); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertVotes stores the Constitutional Committee votes among those cast on
// proposalID, returning the number written. Non-CC votes (DReps, SPOs) are
// ignored — Cella is a committee tool.
func (d *DB) UpsertVotes(proposalID string, votes []koios.Vote) (int, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
INSERT INTO cc_votes (proposal_id, cc_hot_id, vote, rationale_url, block_time)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(proposal_id, cc_hot_id) DO UPDATE SET
  vote=excluded.vote, rationale_url=excluded.rationale_url, block_time=excluded.block_time`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	n := 0
	for _, v := range votes {
		if v.VoterRole != "ConstitutionalCommittee" {
			continue
		}
		if _, err := stmt.Exec(proposalID, v.VoterID, v.Vote, v.MetaURL, v.BlockTime); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

// VoteRow is a stored CC vote.
type VoteRow struct {
	ProposalID   string
	VoterID      string
	Vote         string
	RationaleURL string
	BlockTime    int64
}

// VotesFor returns CC votes for the given proposal IDs, grouped by proposal_id.
func (d *DB) VotesFor(ids []string) (map[string][]VoteRow, error) {
	out := map[string][]VoteRow{}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := d.sql.Query(`
SELECT proposal_id, cc_hot_id, vote, rationale_url, block_time
FROM cc_votes
WHERE proposal_id IN (`+placeholders+`)
ORDER BY cc_hot_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r VoteRow
		var rationale sql.NullString
		if err := rows.Scan(&r.ProposalID, &r.VoterID, &r.Vote, &rationale, &r.BlockTime); err != nil {
			return nil, err
		}
		r.RationaleURL = rationale.String
		out[r.ProposalID] = append(out[r.ProposalID], r)
	}
	return out, rows.Err()
}

// ReviewRow is a stored constitutionality review.
type ReviewRow struct {
	ProposalID string
	Verdict    string
	Summary    string
	Model      string
}

// UpsertReview stores (or replaces) the AI-assisted constitutionality review
// for a governance action.
func (d *DB) UpsertReview(proposalID, verdict, summary, model string) error {
	_, err := d.sql.Exec(`
INSERT INTO reviews (proposal_id, verdict, summary, model, reviewed_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(proposal_id) DO UPDATE SET
  verdict=excluded.verdict, summary=excluded.summary,
  model=excluded.model, reviewed_at=excluded.reviewed_at`,
		proposalID, verdict, summary, model, time.Now().Unix())
	return err
}

// ReviewsFor returns reviews for the given proposal IDs, keyed by proposal_id.
func (d *DB) ReviewsFor(ids []string) (map[string]ReviewRow, error) {
	out := map[string]ReviewRow{}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := d.sql.Query(`
SELECT proposal_id, verdict, summary, COALESCE(model, '')
FROM reviews
WHERE proposal_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r ReviewRow
		if err := rows.Scan(&r.ProposalID, &r.Verdict, &r.Summary, &r.Model); err != nil {
			return nil, err
		}
		out[r.ProposalID] = r
	}
	return out, rows.Err()
}
