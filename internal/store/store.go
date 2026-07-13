// Package store is Cella's local persistence layer, backed by SQLite via a
// pure-Go driver (modernc.org/sqlite). No CGO is required, so `go build`
// produces a single self-contained binary with no external database server.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
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

CREATE TABLE IF NOT EXISTS member_votes (
  proposal_id TEXT NOT NULL,
  member      TEXT NOT NULL,    -- the delegate's identity (signed-in member)
  vote        TEXT,             -- Yes | No | Abstain
  rationale   TEXT,
  updated_at  INTEGER,
  PRIMARY KEY (proposal_id, member)
);

-- The network's genesis parameters, captured at ingest. They turn a governance
-- action's expiration epoch into a wall-clock deadline. Storing them keeps the
-- web server free of network calls.
CREATE TABLE IF NOT EXISTS network (
  id           INTEGER PRIMARY KEY CHECK (id = 1),  -- single row
  system_start INTEGER,
  epoch_length INTEGER,
  fetched_at   INTEGER
);

-- The committee's final, citable rationale for its vote — the body of the
-- CIP-136 document that is anchored on-chain alongside the vote.
CREATE TABLE IF NOT EXISTS rationales (
  proposal_id      TEXT PRIMARY KEY,
  summary          TEXT,        -- <= 300 chars (CIP-136)
  statement        TEXT,        -- the full rationale (markdown)
  precedent        TEXT,
  counterargument  TEXT,
  conclusion       TEXT,
  authored_by      TEXT,        -- the delegate who last edited it
  updated_at       INTEGER
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
	// Idempotent upgrades for databases created before a column existed; a
	// duplicate-column error just means it is already present.
	for _, stmt := range []string{
		`ALTER TABLE governance_actions ADD COLUMN abstract TEXT`,
		`ALTER TABLE member_votes ADD COLUMN signature TEXT`,
		`ALTER TABLE member_votes ADD COLUMN pubkey TEXT`,
	} {
		if _, err := sdb.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			// Non-fatal: a fresh database already has the column.
			_ = err
		}
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
	TxHash     string
	Idx        int
	Type       string
	Title      string
	Abstract   string
	MetaURL    string
	BlockTime  int64
	Expiration sql.NullInt64
}

// Slug is a URL-safe identifier for the action's detail page. The bech32/hash
// proposal_id can contain '#', which is unsafe in a URL path, so we key detail
// pages on tx_hash + cert index instead.
func (r ActionRow) Slug() string {
	return r.TxHash + "-" + strconv.Itoa(r.Idx)
}

// GovID is the on-chain governance-action id used by explorers (AdaStat,
// Cardanoscan): the tx hash followed by the cert index as two hex digits.
func (r ActionRow) GovID() string {
	return r.TxHash + fmt.Sprintf("%02x", r.Idx)
}

// Actions returns stored governance actions, newest first.
func (d *DB) Actions(limit int) ([]ActionRow, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.sql.Query(`
SELECT proposal_id, tx_hash, idx, type, title, COALESCE(abstract, ''), meta_url, block_time, expiration
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
		if err := rows.Scan(&r.ProposalID, &r.TxHash, &r.Idx, &r.Type, &r.Title, &r.Abstract, &r.MetaURL, &r.BlockTime, &r.Expiration); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ActionBySlug returns a single governance action by its URL slug
// (tx_hash + "-" + cert index). The bool is false when no action matches.
func (d *DB) ActionBySlug(slug string) (ActionRow, bool, error) {
	var r ActionRow
	i := strings.LastIndex(slug, "-")
	if i < 0 {
		return r, false, nil
	}
	idx, err := strconv.Atoi(slug[i+1:])
	if err != nil {
		return r, false, nil
	}
	txHash := slug[:i]

	row := d.sql.QueryRow(`
SELECT proposal_id, tx_hash, idx, type, title, COALESCE(abstract, ''), meta_url, block_time, expiration
FROM governance_actions
WHERE tx_hash = ? AND idx = ?`, txHash, idx)
	err = row.Scan(&r.ProposalID, &r.TxHash, &r.Idx, &r.Type, &r.Title, &r.Abstract, &r.MetaURL, &r.BlockTime, &r.Expiration)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	return r, true, nil
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

// MemberVote is a body delegate's internal position on an action.
//
// Signature and PubKey are present when the delegate signed the position with
// their wallet, which is what makes it attributable to them rather than merely
// to whoever held their session. An unsigned vote is still a vote — a demo
// instance has no wallets — but the two are never conflated in the UI.
type MemberVote struct {
	Member    string
	Vote      string
	Rationale string
	Signature string // hex COSE_Sign1 over the vote message, or ""
	PubKey    string // hex COSE_Key of the signer, or ""
}

// Signed reports whether the delegate signed this position with their wallet.
func (m MemberVote) Signed() bool { return m.Signature != "" }

// UpsertMemberVote records (or replaces) a delegate's internal vote + rationale.
// A signature (which may be empty) is stored with it: re-recording a position
// without signing replaces the signature too, because a signature over an old
// position says nothing about the new one.
func (d *DB) UpsertMemberVote(proposalID, member, vote, rationale, signature, pubkey string) error {
	_, err := d.sql.Exec(`
INSERT INTO member_votes (proposal_id, member, vote, rationale, signature, pubkey, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(proposal_id, member) DO UPDATE SET
  vote=excluded.vote, rationale=excluded.rationale,
  signature=excluded.signature, pubkey=excluded.pubkey,
  updated_at=excluded.updated_at`,
		proposalID, member, vote, rationale, signature, pubkey, time.Now().Unix())
	return err
}

// MemberVotesFor returns delegates' internal votes for an action, keyed by member.
func (d *DB) MemberVotesFor(proposalID string) (map[string]MemberVote, error) {
	out := map[string]MemberVote{}
	rows, err := d.sql.Query(`
SELECT member, COALESCE(vote,''), COALESCE(rationale,''), COALESCE(signature,''), COALESCE(pubkey,'')
FROM member_votes WHERE proposal_id = ?`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m MemberVote
		if err := rows.Scan(&m.Member, &m.Vote, &m.Rationale, &m.Signature, &m.PubKey); err != nil {
			return nil, err
		}
		out[m.Member] = m
	}
	return out, rows.Err()
}

// MemberVotesForAll returns delegates' internal votes for many actions at once,
// keyed by proposal_id then member. The index needs a quorum count for every
// action on the page; querying per action would be a query per row.
func (d *DB) MemberVotesForAll(ids []string) (map[string]map[string]MemberVote, error) {
	out := map[string]map[string]MemberVote{}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := d.sql.Query(`
SELECT proposal_id, member, COALESCE(vote,''), COALESCE(rationale,''), COALESCE(signature,'')
FROM member_votes
WHERE proposal_id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pid string
		var m MemberVote
		if err := rows.Scan(&pid, &m.Member, &m.Vote, &m.Rationale, &m.Signature); err != nil {
			return nil, err
		}
		if out[pid] == nil {
			out[pid] = map[string]MemberVote{}
		}
		out[pid][m.Member] = m
	}
	return out, rows.Err()
}

// MemberVotesByMember returns one member's internal votes across all actions,
// keyed by proposal_id (for showing "your vote" on the actions index).
func (d *DB) MemberVotesByMember(member string) (map[string]MemberVote, error) {
	out := map[string]MemberVote{}
	if member == "" {
		return out, nil
	}
	rows, err := d.sql.Query(`
SELECT proposal_id, COALESCE(vote,''), COALESCE(rationale,'')
FROM member_votes WHERE member = ?`, member)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pid string
		var m MemberVote
		m.Member = member
		if err := rows.Scan(&pid, &m.Vote, &m.Rationale); err != nil {
			return nil, err
		}
		out[pid] = m
	}
	return out, rows.Err()
}

// SaveNetwork records the network's genesis parameters.
func (d *DB) SaveNetwork(p koios.GenesisParams) error {
	_, err := d.sql.Exec(`
INSERT INTO network (id, system_start, epoch_length, fetched_at)
VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  system_start=excluded.system_start, epoch_length=excluded.epoch_length,
  fetched_at=excluded.fetched_at`,
		p.SystemStart, p.EpochLength, time.Now().Unix())
	return err
}

// Network returns the stored genesis parameters. They are absent until an
// ingest has run, in which case the returned params are not Valid() and callers
// must fall back to showing the raw expiration epoch rather than inventing a
// date from nothing.
func (d *DB) Network() (koios.GenesisParams, error) {
	var p koios.GenesisParams
	row := d.sql.QueryRow(`SELECT COALESCE(system_start,0), COALESCE(epoch_length,0) FROM network WHERE id = 1`)
	err := row.Scan(&p.SystemStart, &p.EpochLength)
	if err == sql.ErrNoRows {
		return koios.GenesisParams{}, nil
	}
	if err != nil {
		return koios.GenesisParams{}, err
	}
	return p, nil
}

// Rationale is the committee's authored rationale for its vote on an action —
// the reasoning that becomes the body of the anchored CIP-136 document.
type Rationale struct {
	Summary         string
	Statement       string
	Precedent       string
	Counterargument string
	Conclusion      string
	AuthoredBy      string
	UpdatedAt       int64
}

// Empty reports whether nothing has been authored yet.
func (r Rationale) Empty() bool {
	return strings.TrimSpace(r.Summary) == "" && strings.TrimSpace(r.Statement) == ""
}

// UpsertRationale records (or replaces) the committee's rationale for an action.
func (d *DB) UpsertRationale(proposalID string, r Rationale) error {
	_, err := d.sql.Exec(`
INSERT INTO rationales
  (proposal_id, summary, statement, precedent, counterargument, conclusion, authored_by, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(proposal_id) DO UPDATE SET
  summary=excluded.summary, statement=excluded.statement, precedent=excluded.precedent,
  counterargument=excluded.counterargument, conclusion=excluded.conclusion,
  authored_by=excluded.authored_by, updated_at=excluded.updated_at`,
		proposalID, r.Summary, r.Statement, r.Precedent, r.Counterargument, r.Conclusion,
		r.AuthoredBy, time.Now().Unix())
	return err
}

// RationaleFor returns the committee's rationale for an action. The bool is
// false when none has been authored.
func (d *DB) RationaleFor(proposalID string) (Rationale, bool, error) {
	var r Rationale
	row := d.sql.QueryRow(`
SELECT COALESCE(summary,''), COALESCE(statement,''), COALESCE(precedent,''),
       COALESCE(counterargument,''), COALESCE(conclusion,''), COALESCE(authored_by,''),
       COALESCE(updated_at,0)
FROM rationales WHERE proposal_id = ?`, proposalID)
	err := row.Scan(&r.Summary, &r.Statement, &r.Precedent, &r.Counterargument,
		&r.Conclusion, &r.AuthoredBy, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	return r, true, nil
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
