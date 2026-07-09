package server

import "hash/fnv"

// MemberStance is a body member's internal position on a governance action:
// their vote and a short rationale. For the demo these are generated
// deterministically per action so the chamber shows a stable, plausible
// deliberation — illustrating how delegates deliberate *before* the committee
// casts its single on-chain vote. Not real votes.
type MemberStance struct {
	Member
	Vote      string // Yes | No | Abstain
	Rationale string
}

var demoRationales = map[string][]string{
	"Yes": {
		"Consistent with the Constitution's treasury guardrails; the request is proportionate and well-scoped.",
		"No conflict with the guiding principles, and the anchored rationale is sufficient and verifiable.",
		"Serves the ecosystem's long-term interest and respects the defined process — I support it.",
		"Precedent from prior actions supports approval; the safeguards here are adequate.",
	},
	"No": {
		"The action exceeds the limits the Constitution sets for this category.",
		"Anchored metadata is thin — I can't verify compliance with the relevant articles.",
		"Process concerns: the required notice and deliberation window were not observed.",
		"This would weaken the guardrails and set an unfavourable precedent.",
	},
	"Abstain": {
		"Recusing — a potential conflict of interest with a party named in the action.",
		"Insufficient detail to judge; I'd want clarification from the proposer first.",
		"As written, this falls outside the committee's constitutional remit.",
	},
}

// deliberate returns a deterministic demo deliberation for an action: each
// member's stance + rationale, keyed off the proposal id so it's stable.
func deliberate(proposalID string, members []Member) []MemberStance {
	out := make([]MemberStance, 0, len(members))
	for i, m := range members {
		h := fnv.New32a()
		_, _ = h.Write([]byte(proposalID))
		_, _ = h.Write([]byte{byte(i), byte(len(proposalID))})
		n := h.Sum32()

		var vote string
		switch n % 6 {
		case 0, 1, 2:
			vote = "Yes"
		case 3, 4:
			vote = "No"
		default:
			vote = "Abstain"
		}
		rs := demoRationales[vote]
		out = append(out, MemberStance{Member: m, Vote: vote, Rationale: rs[int(n>>3)%len(rs)]})
	}
	return out
}
