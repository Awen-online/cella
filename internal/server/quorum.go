package server

import "github.com/Awen-online/cella/internal/store"

// Quorum is what the chain requires to accept the committee's vote, set against
// what the body has actually produced.
//
// The numbers come from the hot NFT datum, never from Cella's own roster. That
// distinction is the whole point: a Cella that computed quorum for itself could
// display "quorum reached" while the validator rejected the transaction, and a
// committee that believed it would miss its deadline discovering otherwise.
type Quorum struct {
	Known bool // false until an ingest has read the hot NFT datum

	Need int // signatures the validator requires: ceil(n/2) of the voting group
	Have int // voting-group delegates who have signed a position
	Size int // delegates in the voting group

	// Signers and Missing name the voting group's delegates by roster name where
	// Cella can, and by key hash where it cannot — a key in the datum that
	// matches nobody on the roster is a real condition worth showing, not an
	// error to swallow.
	Signers []string
	Missing []string
}

// Met reports whether enough of the voting group has signed.
func (q Quorum) Met() bool { return q.Known && q.Have >= q.Need }

// quorumFor sets the chain's voting group against the delegates who have signed
// a position on this action.
//
// Only *signed* positions count. An unsigned one is a statement of intent
// recorded in a database; it is not a signature the chain will accept, and
// counting it towards quorum would tell a committee it can submit when it
// cannot.
func (s *Server) quorumFor(proposalID string) (Quorum, error) {
	group, err := s.db.VotingGroup()
	if err != nil {
		return Quorum{}, err
	}
	if len(group) == 0 {
		return Quorum{}, nil // no datum read yet — say nothing rather than guess
	}

	votes, err := s.db.MemberVotesFor(proposalID)
	if err != nil {
		return Quorum{}, err
	}

	q := Quorum{
		Known: true,
		Need:  group.Quorum(),
		Size:  len(group.Distinct()),
	}

	for _, keyHash := range group.Distinct() {
		name := s.nameForVotingKey(keyHash)
		if signedBy(votes, name) {
			q.Have++
			q.Signers = append(q.Signers, name)
		} else {
			q.Missing = append(q.Missing, name)
		}
	}
	return q, nil
}

// nameForVotingKey maps an on-chain voting credential to a delegate's name,
// falling back to the key hash itself when the roster does not claim it.
func (s *Server) nameForVotingKey(keyHash string) string {
	for _, m := range s.body.Members {
		if m.VoteKeyHash != "" && m.VoteKeyHash == keyHash {
			return m.Name
		}
	}
	return keyHash
}

// signedBy reports whether the named delegate has signed a position.
func signedBy(votes map[string]store.MemberVote, name string) bool {
	v, ok := votes[name]
	return ok && v.Signed()
}
