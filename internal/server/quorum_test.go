package server

import (
	"crypto/ed25519"
	"testing"

	"github.com/Awen-online/cella/internal/cardano"
)

// quorumServer wires a server whose roster delegates carry on-chain voting key
// hashes, and whose stored voting group is those same three keys.
func quorumServer(t *testing.T, pub ed25519.PublicKey) *Server {
	t.Helper()
	s, _ := seedServer(t)

	const (
		k1 = "c6731b9c6de6bf11d91f08099953cb393505806ff522e5cc3a7574ab"
		k2 = "c6d6ffd8e93b1b8352c297d528c958b982098dc8a08025bbb8d864cf"
		k3 = "2faaa04cee79d9abfa3149c814617e860567a8609bbfbd044566a5cd"
	)
	s.body = Body{
		Name: "Test Body",
		Members: []Member{
			{Name: "Junia Marcia", Address: rewardAddress(t, pub), VoteKeyHash: k1},
			{Name: "Titus Varo", VoteKeyHash: k2},
			{Name: "Cassius Aurel", VoteKeyHash: k3},
		},
	}
	if err := s.db.SaveVotingGroup(cardano.VotingGroup{
		{KeyHash: k1}, {KeyHash: k2}, {KeyHash: k3},
	}); err != nil {
		t.Fatalf("save voting group: %v", err)
	}
	return s
}

// With no datum read, Cella must not claim to know what quorum is. Guessing
// would be worse than silence: a committee could believe it can submit when the
// validator would reject the transaction.
func TestQuorumUnknownWithoutDatum(t *testing.T) {
	s, _ := seedServer(t) // no voting group stored
	_, pid := slugOf(s, t)

	q, err := s.quorumFor(pid)
	if err != nil {
		t.Fatalf("quorumFor: %v", err)
	}
	if q.Known {
		t.Error("quorum claims to be known with no hot NFT datum")
	}
	if q.Met() {
		t.Error("an unknown quorum reported itself met")
	}
	if q.Need != 0 || q.Size != 0 {
		t.Errorf("unknown quorum invented numbers: %+v", q)
	}
}

// Quorum comes from the chain: three voting delegates need two signatures.
func TestQuorumComesFromTheDatum(t *testing.T) {
	pub, _ := newKeypair(t)
	s := quorumServer(t, pub)
	_, pid := slugOf(s, t)

	q, err := s.quorumFor(pid)
	if err != nil {
		t.Fatalf("quorumFor: %v", err)
	}
	if !q.Known {
		t.Fatal("quorum should be known once the datum is stored")
	}
	if q.Size != 3 {
		t.Errorf("voting group size = %d, want 3", q.Size)
	}
	if q.Need != 2 {
		t.Errorf("quorum = %d, want 2 (ceil(3/2))", q.Need)
	}
	if q.Have != 0 || q.Met() {
		t.Errorf("nobody has signed, yet quorum reports %d/%d met=%v", q.Have, q.Need, q.Met())
	}
	if len(q.Missing) != 3 {
		t.Errorf("Missing = %v, want all 3 delegates", q.Missing)
	}
}

// Only *signed* positions count towards quorum. An unsigned one is a database
// row; it is not a signature the chain will accept.
func TestUnsignedPositionsDoNotCountTowardsQuorum(t *testing.T) {
	pub, priv := newKeypair(t)
	s := quorumServer(t, pub)
	slug, pid := slugOf(s, t)

	// Titus Varo records a position but does not sign it.
	if err := s.db.UpsertMemberVote(pid, "Titus Varo", "Yes", "Fine by me.", "", ""); err != nil {
		t.Fatalf("seed unsigned vote: %v", err)
	}
	q, err := s.quorumFor(pid)
	if err != nil {
		t.Fatalf("quorumFor: %v", err)
	}
	if q.Have != 0 {
		t.Errorf("an unsigned position counted towards quorum (Have=%d)", q.Have)
	}
	if q.Met() {
		t.Error("quorum reported met on an unsigned position")
	}

	// Junia Marcia signs hers. Now one real signature exists.
	sig, key := signVote(t, s, priv, pid, "Yes", "Proportionate.")
	if rec := castSigned(t, s, slug, "Junia Marcia", "Yes", "Proportionate.", sig, key); rec.Code != 302 {
		t.Fatalf("signed vote = %d, want 302", rec.Code)
	}

	q, err = s.quorumFor(pid)
	if err != nil {
		t.Fatalf("quorumFor: %v", err)
	}
	if q.Have != 1 {
		t.Errorf("Have = %d, want 1 (only the signed position)", q.Have)
	}
	if q.Met() {
		t.Error("quorum of 2 reported met with a single signature")
	}
	if len(q.Signers) != 1 || q.Signers[0] != "Junia Marcia" {
		t.Errorf("Signers = %v, want [Junia Marcia]", q.Signers)
	}
}

// A voting key in the datum that no roster delegate claims is shown as the key
// hash, not silently dropped — a delegate Cella cannot name is still a delegate
// whose signature the chain requires.
func TestUnclaimedVotingKeyIsStillCounted(t *testing.T) {
	pub, _ := newKeypair(t)
	s := quorumServer(t, pub)
	_, pid := slugOf(s, t)

	// Strip the roster's knowledge of one voting key.
	s.body.Members[2].VoteKeyHash = ""

	q, err := s.quorumFor(pid)
	if err != nil {
		t.Fatalf("quorumFor: %v", err)
	}
	if q.Size != 3 || q.Need != 2 {
		t.Errorf("the voting group shrank because the roster lost a name: %+v", q)
	}
	const orphan = "2faaa04cee79d9abfa3149c814617e860567a8609bbfbd044566a5cd"
	var found bool
	for _, m := range q.Missing {
		if m == orphan {
			found = true
		}
	}
	if !found {
		t.Errorf("an unclaimed voting key was not shown by its key hash; Missing = %v", q.Missing)
	}
}
