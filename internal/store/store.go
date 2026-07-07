// Package store is Cella's local persistence layer, backed by SQLite via a
// pure-Go driver (modernc.org/sqlite). No CGO is required, so `go build`
// produces a single self-contained binary with no external database server.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
  (proposal_id, tx_hash, idx, type, title, meta_url, block_time, expiration, raw, ingested_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(proposal_id) DO UPDATE SET
  type=excluded.type, title=excluded.title, meta_url=excluded.meta_url,
  block_time=excluded.block_time, expiration=excluded.expiration,
  raw=excluded.raw, ingested_at=excluded.ingested_at`)
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
		if _, err := stmt.Exec(a.ProposalID, a.TxHash, a.Index, a.Type, a.Title(), a.MetaURL, a.BlockTime, exp, string(raw), now); err != nil {
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
SELECT proposal_id, type, title, meta_url, block_time, expiration
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
		if err := rows.Scan(&r.ProposalID, &r.Type, &r.Title, &r.MetaURL, &r.BlockTime, &r.Expiration); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
