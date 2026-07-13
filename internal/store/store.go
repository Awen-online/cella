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

	"github.com/Awen-online/cella/internal/cardano"
	"github.com/Awen-online/cella/internal/govaction"
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

-- The hot NFT datum's voting group: who the chain will accept vote signatures
-- from, and therefore what quorum actually is. Read from the chain at ingest,
-- never invented locally — a quorum Cella computed for itself could disagree
-- with the validator, which is worse than showing none.
CREATE TABLE IF NOT EXISTS voting_group (
  key_hash  TEXT PRIMARY KEY,
  cert_hash TEXT,
  position  INTEGER,
  synced_at INTEGER
);

-- The Constitutional Committee as the chain records it: who holds a seat, who
-- has resigned, when each term ends. Read from /committee_info, never hardcoded
-- — a static roster is quietly wrong the first time anyone resigns.
CREATE TABLE IF NOT EXISTS committee (
  cc_hot_id        TEXT PRIMARY KEY,
  cc_cold_id       TEXT,
  status           TEXT,        -- authorized | resigned | not_authorized
  expiration_epoch INTEGER,
  synced_at        INTEGER
);

-- Coordination flags: a delegate raising a hand to the rest of the chamber.
-- One row per delegate per flag, so a delegate raising a concern cannot silently
-- overwrite a colleague's — the chamber needs to see who is saying what, not
-- just that somebody once said it.
CREATE TABLE IF NOT EXISTS chamber_flags (
  proposal_id TEXT NOT NULL,
  member      TEXT NOT NULL,
  flag        TEXT NOT NULL,   -- discuss | ready | blocked
  raised_at   INTEGER,
  PRIMARY KEY (proposal_id, member, flag)
);

-- A delegate's private working notes on an action. Nobody else can read them,
-- and they are not a position: thinking out loud is not the same as taking a
-- stand, and a delegate who cannot do the former in private will do less of it.
CREATE TABLE IF NOT EXISTS drafts (
  proposal_id TEXT NOT NULL,
  member      TEXT NOT NULL,
  body        TEXT,
  updated_at  INTEGER,
  PRIMARY KEY (proposal_id, member)
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

		// What the action actually does, and what became of it.
		`ALTER TABLE governance_actions ADD COLUMN description TEXT`,
		`ALTER TABLE governance_actions ADD COLUMN status TEXT`,
		`ALTER TABLE governance_actions ADD COLUMN motivation TEXT`,
		`ALTER TABLE governance_actions ADD COLUMN proposer_rationale TEXT`,
		`ALTER TABLE governance_actions ADD COLUMN deposit TEXT`,
		`ALTER TABLE governance_actions ADD COLUMN return_address TEXT`,
		`ALTER TABLE governance_actions ADD COLUMN proposed_epoch INTEGER`,
		`ALTER TABLE governance_actions ADD COLUMN meta_hash TEXT`,
		`ALTER TABLE governance_actions ADD COLUMN meta_is_valid INTEGER`,

		// The ecosystem's stake-weighted verdict, as context for the committee's.
		`ALTER TABLE governance_actions ADD COLUMN voting_summary TEXT`,

		// The committee's own ratification threshold, as the chain sets it.
		`ALTER TABLE network ADD COLUMN quorum_numerator INTEGER`,
		`ALTER TABLE network ADD COLUMN quorum_denominator INTEGER`,
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
  (proposal_id, tx_hash, idx, type, title, abstract, meta_url, block_time, expiration, raw, ingested_at,
   description, status, motivation, proposer_rationale, deposit, return_address, proposed_epoch,
   meta_hash, meta_is_valid)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(proposal_id) DO UPDATE SET
  type=excluded.type, title=excluded.title, abstract=excluded.abstract,
  meta_url=excluded.meta_url, block_time=excluded.block_time,
  expiration=excluded.expiration, raw=excluded.raw, ingested_at=excluded.ingested_at,
  description=excluded.description, status=excluded.status, motivation=excluded.motivation,
  proposer_rationale=excluded.proposer_rationale, deposit=excluded.deposit,
  return_address=excluded.return_address, proposed_epoch=excluded.proposed_epoch,
  meta_hash=excluded.meta_hash, meta_is_valid=excluded.meta_is_valid`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	n := 0
	for _, a := range actions {
		raw, _ := json.Marshal(a)

		var exp, proposed, valid any
		if a.Expiration != nil {
			exp = *a.Expiration
		}
		if a.ProposedEpoch != nil {
			proposed = *a.ProposedEpoch
		}
		if a.MetaIsValid != nil {
			valid = *a.MetaIsValid
		}

		if _, err := stmt.Exec(
			a.ProposalID, a.TxHash, a.Index, a.Type, a.Title(), a.Abstract(), a.MetaURL,
			a.BlockTime, exp, string(raw), now,
			string(a.Description), string(a.Status()), a.Motivation(), a.Rationale(),
			a.Deposit.String(), a.ReturnAddress, proposed, a.MetaHash, valid,
		); err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

// SaveVotingSummary records the ecosystem's stake-weighted tally for an action.
func (d *DB) SaveVotingSummary(proposalID string, s koios.VotingSummary) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = d.sql.Exec(`UPDATE governance_actions SET voting_summary = ? WHERE proposal_id = ?`,
		string(raw), proposalID)
	return err
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

	// Status is the action's fate: Live, Ratified, Enacted, Dropped, Expired.
	// A countdown on an action that was enacted last month is a lie.
	Status string

	// Description is the on-chain payload — what the action actually does.
	Description string

	// The proposer's own case for the action (CIP-108), distinct from the
	// committee's rationale, which Cella authors separately.
	Motivation        string
	ProposerRationale string

	Deposit       string // lovelace, as a decimal string
	ReturnAddress string // where the deposit goes back to; effectively the proposer
	ProposedEpoch sql.NullInt64

	// MetaHash is what the chain committed to; MetaValid records whether the
	// document at MetaURL actually hashes to it. Invalid means the abstract a
	// delegate is reading is not the one the proposer signed for.
	MetaHash  string
	MetaValid sql.NullBool

	// VotingSummary is the raw Koios stake-weighted tally, or "".
	VotingSummary string
}

// Live reports whether the action is still accepting votes. An empty status
// means the row predates status tracking, in which case we cannot claim it is
// settled and treat it as live.
func (r ActionRow) Live() bool {
	return r.Status == "" || r.Status == string(koios.StatusLive)
}

// Settled reports whether the chain has already decided this action's fate.
func (r ActionRow) Settled() bool { return !r.Live() }

// Payload decodes what the action does. The bool is false when there is nothing
// decodable — an older row, or an action type Cella has never seen.
func (r ActionRow) Payload() (govaction.Payload, bool) {
	if r.Description == "" {
		return govaction.Payload{}, false
	}
	p, err := govaction.Decode(json.RawMessage(r.Description))
	if err != nil {
		return govaction.Payload{}, false
	}
	return p, true
}

// Summary decodes the ecosystem's stake-weighted tally, if one was recorded.
func (r ActionRow) Summary() (koios.VotingSummary, bool) {
	if r.VotingSummary == "" {
		return koios.VotingSummary{}, false
	}
	var s koios.VotingSummary
	if err := json.Unmarshal([]byte(r.VotingSummary), &s); err != nil {
		return koios.VotingSummary{}, false
	}
	return s, true
}

// actionColumns is the SELECT list every action read shares, so a new column
// cannot be added to one query and forgotten in the other.
const actionColumns = `
  proposal_id, tx_hash, idx, type, title, COALESCE(abstract,''), meta_url, block_time, expiration,
  COALESCE(status,''), COALESCE(description,''), COALESCE(motivation,''),
  COALESCE(proposer_rationale,''), COALESCE(deposit,''), COALESCE(return_address,''),
  proposed_epoch, COALESCE(meta_hash,''), meta_is_valid, COALESCE(voting_summary,'')`

// scanAction reads one row of actionColumns.
func scanAction(sc interface{ Scan(...any) error }) (ActionRow, error) {
	var r ActionRow
	err := sc.Scan(
		&r.ProposalID, &r.TxHash, &r.Idx, &r.Type, &r.Title, &r.Abstract, &r.MetaURL,
		&r.BlockTime, &r.Expiration,
		&r.Status, &r.Description, &r.Motivation, &r.ProposerRationale, &r.Deposit,
		&r.ReturnAddress, &r.ProposedEpoch, &r.MetaHash, &r.MetaValid, &r.VotingSummary,
	)
	return r, err
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
SELECT`+actionColumns+`
FROM governance_actions
ORDER BY block_time DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActionRow
	for rows.Next() {
		r, err := scanAction(rows)
		if err != nil {
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
SELECT`+actionColumns+`
FROM governance_actions
WHERE tx_hash = ? AND idx = ?`, txHash, idx)

	r, err = scanAction(row)
	if err == sql.ErrNoRows {
		return ActionRow{}, false, nil
	}
	if err != nil {
		return ActionRow{}, false, err
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

// SaveVotingGroup replaces the stored voting group with what the chain says.
// It is a replacement, not a merge: a delegate rotated out of the hot NFT datum
// must disappear from Cella too, or Cella would keep counting a signature the
// validator no longer accepts.
func (d *DB) SaveVotingGroup(g cardano.VotingGroup) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM voting_group`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
INSERT INTO voting_group (key_hash, cert_hash, position, synced_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for i, id := range g {
		if _, err := stmt.Exec(id.KeyHash, id.CertHash, i, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// VotingGroup returns the stored voting group, in datum order. It is empty
// until an ingest has read the hot NFT — in which case Cella must not pretend
// to know what quorum is.
func (d *DB) VotingGroup() (cardano.VotingGroup, error) {
	rows, err := d.sql.Query(`
SELECT key_hash, COALESCE(cert_hash,'') FROM voting_group ORDER BY position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var g cardano.VotingGroup
	for rows.Next() {
		var id cardano.VotingIdentity
		if err := rows.Scan(&id.KeyHash, &id.CertHash); err != nil {
			return nil, err
		}
		g = append(g, id)
	}
	return g, rows.Err()
}

// SaveCommittee replaces the stored committee with what the chain reports. It
// is a replacement, not a merge: a seat that has been removed on-chain must
// disappear from Cella too, or the threshold's denominator would be wrong.
func (d *DB) SaveCommittee(c koios.CommitteeInfo) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM committee`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
INSERT INTO committee (cc_hot_id, cc_cold_id, status, expiration_epoch, synced_at)
VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, m := range c.Members {
		var exp any
		if m.ExpirationEpoch != nil {
			exp = *m.ExpirationEpoch
		}
		// A resigned seat has no hot credential; key it on the cold one so it is
		// still recorded rather than colliding on an empty primary key.
		id := m.HotID
		if id == "" {
			id = "resigned:" + m.ColdID
		}
		if _, err := stmt.Exec(id, m.ColdID, m.Status, exp, now); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`
INSERT INTO network (id, quorum_numerator, quorum_denominator)
VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  quorum_numerator=excluded.quorum_numerator,
  quorum_denominator=excluded.quorum_denominator`,
		c.QuorumNumerator, c.QuorumDenominator); err != nil {
		return err
	}
	return tx.Commit()
}

// Committee returns the stored committee and its quorum threshold. The members
// list is empty until an ingest has read it, in which case Cella must not
// pretend to know what the committee looks like.
func (d *DB) Committee() (koios.CommitteeInfo, error) {
	var c koios.CommitteeInfo

	row := d.sql.QueryRow(`
SELECT COALESCE(quorum_numerator,0), COALESCE(quorum_denominator,0) FROM network WHERE id = 1`)
	if err := row.Scan(&c.QuorumNumerator, &c.QuorumDenominator); err != nil && err != sql.ErrNoRows {
		return c, err
	}

	rows, err := d.sql.Query(`
SELECT cc_hot_id, COALESCE(cc_cold_id,''), COALESCE(status,''), expiration_epoch
FROM committee ORDER BY status, cc_hot_id`)
	if err != nil {
		return c, err
	}
	defer rows.Close()

	for rows.Next() {
		var m koios.CommitteeSeat
		var exp sql.NullInt64
		if err := rows.Scan(&m.HotID, &m.ColdID, &m.Status, &exp); err != nil {
			return c, err
		}
		if exp.Valid {
			e := exp.Int64
			m.ExpirationEpoch = &e
		}
		if strings.HasPrefix(m.HotID, "resigned:") {
			m.HotID = ""
		}
		c.Members = append(c.Members, m)
	}
	return c, rows.Err()
}

// Flag is one delegate raising a hand on an action.
type Flag struct {
	Member   string
	Flag     string
	RaisedAt int64
}

// ToggleFlag raises or lowers a delegate's flag, returning whether it is now
// raised. A delegate can only toggle their own — lowering a colleague's flag
// would silence a concern they meant the chamber to see.
func (d *DB) ToggleFlag(proposalID, member, flag string) (bool, error) {
	var exists int
	err := d.sql.QueryRow(`
SELECT COUNT(*) FROM chamber_flags WHERE proposal_id = ? AND member = ? AND flag = ?`,
		proposalID, member, flag).Scan(&exists)
	if err != nil {
		return false, err
	}

	if exists > 0 {
		_, err := d.sql.Exec(`
DELETE FROM chamber_flags WHERE proposal_id = ? AND member = ? AND flag = ?`,
			proposalID, member, flag)
		return false, err
	}
	_, err = d.sql.Exec(`
INSERT INTO chamber_flags (proposal_id, member, flag, raised_at) VALUES (?, ?, ?, ?)`,
		proposalID, member, flag, time.Now().Unix())
	return true, err
}

// FlagsFor returns every flag raised on an action, grouped by flag.
func (d *DB) FlagsFor(proposalID string) (map[string][]Flag, error) {
	out := map[string][]Flag{}
	rows, err := d.sql.Query(`
SELECT member, flag, COALESCE(raised_at,0) FROM chamber_flags
WHERE proposal_id = ? ORDER BY raised_at`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var f Flag
		if err := rows.Scan(&f.Member, &f.Flag, &f.RaisedAt); err != nil {
			return nil, err
		}
		out[f.Flag] = append(out[f.Flag], f)
	}
	return out, rows.Err()
}

// FlagCountsFor returns how many flags of each kind are raised across many
// actions, so the dashboard can show a chamber's outstanding concerns without a
// query per row.
func (d *DB) FlagCountsFor(ids []string) (map[string]map[string]int, error) {
	out := map[string]map[string]int{}
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := d.sql.Query(`
SELECT proposal_id, flag, COUNT(*) FROM chamber_flags
WHERE proposal_id IN (`+placeholders+`) GROUP BY proposal_id, flag`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pid, flag string
		var n int
		if err := rows.Scan(&pid, &flag, &n); err != nil {
			return nil, err
		}
		if out[pid] == nil {
			out[pid] = map[string]int{}
		}
		out[pid][flag] = n
	}
	return out, rows.Err()
}

// SaveDraft records a delegate's private working notes on an action.
func (d *DB) SaveDraft(proposalID, member, body string) error {
	_, err := d.sql.Exec(`
INSERT INTO drafts (proposal_id, member, body, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT(proposal_id, member) DO UPDATE SET
  body=excluded.body, updated_at=excluded.updated_at`,
		proposalID, member, body, time.Now().Unix())
	return err
}

// Draft returns a delegate's private notes on an action. It is scoped to the
// member by the query itself: there is no path by which one delegate's notes
// can be read as another's.
func (d *DB) Draft(proposalID, member string) (string, int64, error) {
	var body string
	var at int64
	err := d.sql.QueryRow(`
SELECT COALESCE(body,''), COALESCE(updated_at,0) FROM drafts
WHERE proposal_id = ? AND member = ?`, proposalID, member).Scan(&body, &at)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	return body, at, err
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
