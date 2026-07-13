package server

import (
	"net/url"
	"strings"

	"github.com/Awen-online/cella/internal/network"
	"github.com/Awen-online/cella/internal/store"
)

// ChainVote is one vote cast by somebody outside the committee — a delegate
// representative or a stake pool operator — prepared for display.
//
// The committee decides constitutionality on its own reading of the
// Constitution, not by following the chain. But a delegate representative who
// publishes a rationale has done the work of saying *why*, often citing the
// same articles the committee is weighing. Reading that reasoning is part of
// deliberating well; counting it is not. So Cella links the rationales and
// leaves the judgement where it belongs.
type ChainVote struct {
	Vote     string // Yes | No | Abstain
	VoterID  string // bech32, as the chain records it
	Short    string // an abbreviation a human can hold in their head
	Explorer string // the voter on a block explorer

	// Rationale is where this voter published their reasoning, resolved to
	// something a browser will open. Empty when they published none, or when
	// what they published is not a URL we are willing to link.
	Rationale string
}

// HasRationale reports whether this voter said why.
func (v ChainVote) HasRationale() bool { return v.Rationale != "" }

// Class is the CSS class for the vote, matching the committee's own tally.
func (v ChainVote) Class() string {
	switch v.Vote {
	case "Yes":
		return "y"
	case "No":
		return "n"
	default:
		return "a"
	}
}

// chainVotes prepares the votes of one non-committee role for display.
func chainVotes(rows []store.ChainVote, net network.Network, role string) []ChainVote {
	out := make([]ChainVote, 0, len(rows))
	for _, r := range rows {
		v := ChainVote{
			Vote:      r.Vote,
			VoterID:   r.VoterID,
			Short:     shortID(r.VoterID),
			Rationale: rationaleLink(r.RationaleURL),
		}
		if role == "SPO" {
			v.Explorer = net.ExplorerPool(r.VoterID)
		} else {
			v.Explorer = net.ExplorerDRep(r.VoterID)
		}
		out = append(out, v)
	}
	return out
}

// rationaleCount is how many of these voters published their reasoning.
func rationaleCount(votes []ChainVote) int {
	n := 0
	for _, v := range votes {
		if v.HasRationale() {
			n++
		}
	}
	return n
}

// rationaleLink turns a metadata anchor into a URL a browser will open, or
// returns "" if it will not.
//
// The anchor is written by the voter and read off the chain, so it is data from
// a stranger, not a URL we chose. Only http, https and ipfs are allowed
// through; a `javascript:` or `data:` anchor is dropped rather than rendered
// into an href. IPFS is rewritten to a public gateway, since a browser cannot
// follow ipfs:// on its own.
func rationaleLink(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return raw
	case "ipfs":
		// ipfs://<cid>[/path] — the CID lands in Host, the rest in Path.
		cid := u.Host + u.Path
		if cid = strings.Trim(cid, "/"); cid == "" {
			return ""
		}
		return "https://ipfs.io/ipfs/" + cid
	default:
		return ""
	}
}

// shortID abbreviates a bech32 identifier: enough of the head and tail to tell
// two voters apart, without a 58-character string swallowing the row.
func shortID(id string) string {
	if len(id) <= 20 {
		return id
	}
	return id[:12] + "…" + id[len(id)-6:]
}
