package koios

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
)

// A Constitutional Committee does not vote in a vacuum. The same action is
// being voted on by delegate representatives and stake pool operators, and how
// *they* are voting is material context for a committee weighing
// constitutionality — not because the committee should follow them, but because
// it should know what it is agreeing or disagreeing with.
//
// Koios gives the whole picture in one call.

// VotingSummary is the stake-weighted tally across all three voter roles.
//
// Vote power is lovelace and routinely exceeds what a float64 can hold exactly,
// so Koios sends it as a string and Cella keeps it as one until it is measured.
type VotingSummary struct {
	ProposalType string `json:"proposal_type"`
	EpochNo      int64  `json:"epoch_no"`

	DRepYesVotes     int     `json:"drep_yes_votes_cast"`
	DRepNoVotes      int     `json:"drep_no_votes_cast"`
	DRepAbstain      int     `json:"drep_abstain_votes_cast"`
	DRepYesPower     string  `json:"drep_yes_vote_power"`
	DRepNoPower      string  `json:"drep_no_vote_power"`
	DRepAbstainPower string  `json:"drep_active_abstain_vote_power"`
	DRepYesPct       float64 `json:"drep_yes_pct"`
	DRepNoPct        float64 `json:"drep_no_pct"`

	PoolYesVotes int     `json:"pool_yes_votes_cast"`
	PoolNoVotes  int     `json:"pool_no_votes_cast"`
	PoolAbstain  int     `json:"pool_abstain_votes_cast"`
	PoolYesPower string  `json:"pool_yes_vote_power"`
	PoolNoPower  string  `json:"pool_no_vote_power"`
	PoolYesPct   float64 `json:"pool_yes_pct"`
	PoolNoPct    float64 `json:"pool_no_pct"`

	CommitteeYesVotes int     `json:"committee_yes_votes_cast"`
	CommitteeNoVotes  int     `json:"committee_no_votes_cast"`
	CommitteeAbstain  int     `json:"committee_abstain_votes_cast"`
	CommitteeYesPct   float64 `json:"committee_yes_pct"`
	CommitteeNoPct    float64 `json:"committee_no_pct"`
}

// DRepVotes is how many delegate representatives have voted at all.
func (s VotingSummary) DRepVotes() int { return s.DRepYesVotes + s.DRepNoVotes + s.DRepAbstain }

// PoolVotes is how many stake pool operators have voted at all.
func (s VotingSummary) PoolVotes() int { return s.PoolYesVotes + s.PoolNoVotes + s.PoolAbstain }

// Power parses a lovelace vote-power string. Anything unparseable is zero — a
// missing number must not silently become a wrong one.
func Power(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return big.NewInt(0)
	}
	return n
}

// ProposalVotingSummary fetches the stake-weighted tally for one action. The
// bool is false when nobody has voted on it yet, which is not an error.
func (c *Client) ProposalVotingSummary(ctx context.Context, proposalID string) (VotingSummary, bool, error) {
	var out VotingSummary

	u := fmt.Sprintf("%s/proposal_voting_summary?_proposal_id=%s", c.baseURL, url.QueryEscape(proposalID))
	body, err := c.get(ctx, u)
	if err != nil {
		return out, false, err
	}

	var rows []VotingSummary
	if err := json.Unmarshal(body, &rows); err != nil {
		return out, false, fmt.Errorf("decode proposal_voting_summary: %w", err)
	}
	if len(rows) == 0 {
		return out, false, nil
	}
	return rows[0], true, nil
}
