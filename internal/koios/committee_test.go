package koios

import "testing"

// With 7 authorized seats and a 2/3 quorum the committee needs 5 Yes votes, not
// 4. Rounding down would have a committee believe it had ratified an action the
// chain rejects — and resigned seats must not pad the denominator.
func TestYesNeededRoundsUpAndExcludesResigned(t *testing.T) {
	seat := func(status string) CommitteeSeat { return CommitteeSeat{Status: status} }

	c := CommitteeInfo{
		QuorumNumerator: 2, QuorumDenominator: 3,
		Members: []CommitteeSeat{
			seat("authorized"), seat("authorized"), seat("authorized"), seat("authorized"),
			seat("authorized"), seat("authorized"), seat("authorized"),
			seat("resigned"), // must not count towards the threshold
		},
	}
	if got := len(c.Authorized()); got != 7 {
		t.Fatalf("authorized seats = %d, want 7 (the resigned seat must be excluded)", got)
	}
	if got := c.YesNeeded(); got != 5 {
		t.Errorf("YesNeeded = %d, want 5 — ceil(7 * 2/3) is 5, not 4", got)
	}
	if got := c.Quorum(); got != "2/3" {
		t.Errorf("Quorum = %q, want 2/3", got)
	}
}

func TestYesNeeded(t *testing.T) {
	cases := []struct{ seats, num, den, want int }{
		{7, 2, 3, 5}, // the live mainnet committee
		{6, 2, 3, 4},
		{3, 2, 3, 2},
		{9, 2, 3, 6},
		{5, 1, 2, 3}, // ceil(2.5)
		{4, 1, 2, 2},
		{0, 2, 3, 0}, // no committee: no threshold to state
	}
	for _, tc := range cases {
		c := CommitteeInfo{QuorumNumerator: tc.num, QuorumDenominator: tc.den}
		for i := 0; i < tc.seats; i++ {
			c.Members = append(c.Members, CommitteeSeat{Status: "authorized"})
		}
		if got := c.YesNeeded(); got != tc.want {
			t.Errorf("%d seats at %d/%d needs %d Yes, want %d", tc.seats, tc.num, tc.den, got, tc.want)
		}
	}
}

// A zero denominator must not divide by zero or invent a threshold.
func TestYesNeededWithNoQuorumRecorded(t *testing.T) {
	c := CommitteeInfo{Members: []CommitteeSeat{{Status: "authorized"}}}
	if got := c.YesNeeded(); got != 0 {
		t.Errorf("YesNeeded = %d with no quorum recorded, want 0", got)
	}
	if c.Quorum() != "" {
		t.Errorf("Quorum = %q, want empty", c.Quorum())
	}
}
