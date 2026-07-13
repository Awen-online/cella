package server

import (
	"fmt"
	"strings"
)

// A recorded vote is only as trustworthy as the session that recorded it — and
// a session is a cookie. Signing the vote itself raises it from "the database
// says Junia Marcia voted No" to "here is Junia Marcia's signature over the
// words 'I vote No'". That difference matters because the tally is published:
// it becomes the internalVote block of the CIP-136 rationale, which is anchored
// on-chain and cited permanently.
//
// The message is built by the server and rebuilt by the server at verification.
// The browser is handed a message to sign, but nothing it sends back about
// *what* was signed is trusted: the signature is checked against bytes the
// server derived from the vote it is about to record. A delegate cannot be
// shown one thing and have another recorded.

// voteMessage is the exact text a delegate signs to record a position. It is
// deliberately plain language: a CIP-30 wallet shows the payload to the signer,
// and a delegate is entitled to read what they are putting their name to rather
// than approve an opaque hash.
//
// Every field that gives the vote meaning is inside the signature — the body,
// the action, the position, and the rationale. Change any of them and the
// signature no longer verifies, so a recorded vote cannot be quietly edited
// afterwards.
func voteMessage(bodyName, proposalID, vote, rationale string) string {
	var b strings.Builder
	b.WriteString("Cella — record a committee position\n\n")
	fmt.Fprintf(&b, "Body:   %s\n", bodyName)
	fmt.Fprintf(&b, "Action: %s\n", proposalID)
	fmt.Fprintf(&b, "Vote:   %s\n", vote)

	rationale = strings.TrimSpace(rationale)
	if rationale == "" {
		b.WriteString("\nRationale: (none given)\n")
	} else {
		fmt.Fprintf(&b, "\nRationale:\n%s\n", rationale)
	}

	b.WriteString("\nSigning records this as your position. No funds move.")
	return b.String()
}
