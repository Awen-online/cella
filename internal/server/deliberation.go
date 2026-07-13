package server

// The chamber is where the body's delegates deliberate before the committee
// casts its single on-chain vote. What it shows is exactly what the delegates
// have recorded — nothing is inferred, generated, or filled in on their behalf.
// A delegate who has not spoken is shown as not having spoken, because a
// committee that cannot tell a real position from a placeholder cannot vote on
// one.

// MemberStance is a delegate's position on a governance action: their vote and
// the rationale they gave for it. Recorded is false when the delegate has not
// taken a position yet, in which case Vote and Rationale are empty.
type MemberStance struct {
	Member
	Vote      string // Yes | No | Abstain — empty until recorded
	Rationale string
	Recorded  bool
}

// chamber returns every delegate on the roster with the position they have
// recorded on the action, in roster order. Delegates who have not voted appear
// with Recorded false rather than being omitted: the committee needs to see who
// is still outstanding.
func (s *Server) chamber(proposalID string) ([]MemberStance, error) {
	votes, err := s.db.MemberVotesFor(proposalID)
	if err != nil {
		return nil, err
	}

	out := make([]MemberStance, 0, len(demoBody.Members))
	for _, m := range demoBody.Members {
		st := MemberStance{Member: m}
		if v, ok := votes[m.Name]; ok && v.Vote != "" {
			st.Vote, st.Rationale, st.Recorded = v.Vote, v.Rationale, true
		}
		out = append(out, st)
	}
	return out, nil
}

// position describes where the chamber stands, in words, for display.
func (t tally) position() string {
	if t.Recorded() == 0 {
		return "No positions recorded yet"
	}
	switch {
	case t.Yes > t.No && t.Yes >= t.Abstain:
		return "Leaning to approve"
	case t.No > t.Yes && t.No >= t.Abstain:
		return "Leaning to reject"
	default:
		return "No consensus yet"
	}
}
