package koios

import (
	"context"
	"encoding/json"
	"fmt"
)

// Who sits on the Constitutional Committee, and what it takes for them to
// agree, are facts about the chain — not facts about Cella's configuration.
// Members resign, terms expire, and a committee update can change the quorum
// itself. A hardcoded roster is a roster that is quietly wrong the first time
// any of that happens, and a committee that cannot see its own threshold cannot
// know whether it has met it.

// CommitteeInfo is the Constitutional Committee as the chain currently records
// it.
type CommitteeInfo struct {
	QuorumNumerator   int             `json:"quorum_numerator"`
	QuorumDenominator int             `json:"quorum_denominator"`
	Members           []CommitteeSeat `json:"members"`
}

// CommitteeSeat is one seat on the committee.
type CommitteeSeat struct {
	HotID           string `json:"cc_hot_id"`
	ColdID          string `json:"cc_cold_id"`
	Status          string `json:"status"` // authorized | resigned | not_authorized
	ExpirationEpoch *int64 `json:"expiration_epoch"`
}

// Authorized reports whether this seat may currently vote. A resigned member is
// still on the roster the chain returns, but their vote no longer counts and
// they must not be counted in the denominator of a threshold.
func (s CommitteeSeat) Authorized() bool { return s.Status == "authorized" }

// Authorized returns only the seats that may vote.
func (c CommitteeInfo) Authorized() []CommitteeSeat {
	out := make([]CommitteeSeat, 0, len(c.Members))
	for _, m := range c.Members {
		if m.Authorized() {
			out = append(out, m)
		}
	}
	return out
}

// YesNeeded is how many authorized seats must vote Yes for the committee to
// ratify, being the quorum fraction of the authorized seats, rounded up.
//
// With 7 authorized seats and a 2/3 quorum that is 5, not 4: a committee that
// rounded down would believe it had ratified an action the chain rejects.
func (c CommitteeInfo) YesNeeded() int {
	n := len(c.Authorized())
	if n == 0 || c.QuorumDenominator == 0 {
		return 0
	}
	num := n * c.QuorumNumerator
	den := c.QuorumDenominator
	needed := num / den
	if num%den != 0 {
		needed++
	}
	return needed
}

// Quorum renders the threshold as the chain states it, e.g. "2/3".
func (c CommitteeInfo) Quorum() string {
	if c.QuorumDenominator == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", c.QuorumNumerator, c.QuorumDenominator)
}

// Committee fetches the Constitutional Committee's current composition and
// quorum threshold.
func (c *Client) Committee(ctx context.Context) (CommitteeInfo, error) {
	var out CommitteeInfo

	body, err := c.get(ctx, c.baseURL+"/committee_info")
	if err != nil {
		return out, err
	}

	var rows []CommitteeInfo
	if err := json.Unmarshal(body, &rows); err != nil {
		return out, fmt.Errorf("decode committee_info: %w", err)
	}
	if len(rows) == 0 {
		return out, fmt.Errorf("koios returned no committee")
	}
	if len(rows[0].Members) == 0 {
		return out, fmt.Errorf("koios returned a committee with no seats")
	}
	return rows[0], nil
}
