package cardano

import (
	"encoding/hex"
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

// The hot NFT datum is where a Constitutional Committee consortium records who
// may cast its votes. Cella reads it rather than trusting its own configuration,
// because the chain is the only authority on the matter: a Cella that computed
// quorum from a local roster could cheerfully report "quorum reached" while the
// validator disagreed, which is worse than reporting nothing at all.
//
// Structure (IntersectMBO/credential-manager, CredentialManager.Api):
//
//	newtype HotLockDatum = HotLockDatum { votingUsers :: [Identity] }
//	data Identity = Identity { pubKeyHash :: PubKeyHash, certificateHash :: CertificateHash }
//
// HotLockDatum's ToData is newtype-derived, so on-chain it is a *bare* list —
// there is no enclosing constructor. (The project's own documentation describes
// a constructor wrapper here; the documentation is stale, and a decoder written
// to it fails against every real UTxO.) Each Identity is Constr 0 of two byte
// strings: a 28-byte blake2b-224 key hash and a 32-byte SHA-256 X.509
// certificate hash.

// constrTag is the CBOR tag Plutus uses for constructor 0.
const constrTag = 121

// VotingIdentity is one delegate authorized to cast the committee's vote.
type VotingIdentity struct {
	KeyHash  string // hex, 28 bytes — signs the vote transaction
	CertHash string // hex, 32 bytes — the X.509 certificate committing to that key
}

// VotingGroup is the hot NFT datum's voting users: exactly who the chain will
// accept signatures from.
type VotingGroup []VotingIdentity

// Quorum is how many of the group's signatures the validator requires.
//
// It is ceil(n/2), computed over distinct key hashes — the validator dedupes
// the group before counting. Note this is *half, rounded up*, not "more than
// half": a group of four needs two signatures, not three. Guessing
// floor(n/2)+1, the more intuitive rule, would have Cella tell a committee it
// was one signature short of a quorum it had already reached.
func (g VotingGroup) Quorum() int {
	n := len(g.Distinct())
	if n == 0 {
		return 0
	}
	return n/2 + n%2
}

// Distinct returns the group's unique key hashes, in order of first appearance.
func (g VotingGroup) Distinct() []string {
	seen := make(map[string]bool, len(g))
	out := make([]string, 0, len(g))
	for _, id := range g {
		if !seen[id.KeyHash] {
			seen[id.KeyHash] = true
			out = append(out, id.KeyHash)
		}
	}
	return out
}

// Has reports whether a key hash is in the voting group.
func (g VotingGroup) Has(keyHash string) bool {
	for _, id := range g {
		if id.KeyHash == keyHash {
			return true
		}
	}
	return false
}

// DecodeHotDatum reads the voting group out of a hot NFT inline datum, given
// its raw CBOR as hex.
//
// Decoding — as opposed to re-encoding — is indifferent to whether the arrays
// are definite or indefinite length, which is fortunate: Plutus emits
// indefinite-length arrays here, and a decoder that insisted on definite ones
// would reject every real datum.
func DecodeHotDatum(cborHex string) (VotingGroup, error) {
	raw, err := hex.DecodeString(cborHex)
	if err != nil {
		return nil, fmt.Errorf("hot datum is not hex: %w", err)
	}

	// The datum is a bare list of tagged Identity constructors.
	var tags []cbor.RawTag
	if err := cbor.Unmarshal(raw, &tags); err != nil {
		return nil, fmt.Errorf("hot datum is not a list of constructors: %w", err)
	}

	group := make(VotingGroup, 0, len(tags))
	for i, t := range tags {
		if t.Number != constrTag {
			return nil, fmt.Errorf("voting user %d has CBOR tag %d, want %d (Plutus constructor 0)",
				i, t.Number, constrTag)
		}

		var fields struct {
			_        struct{} `cbor:",toarray"`
			KeyHash  []byte
			CertHash []byte
		}
		if err := cbor.Unmarshal(t.Content, &fields); err != nil {
			return nil, fmt.Errorf("voting user %d: %w", i, err)
		}
		if len(fields.KeyHash) != KeyHashLen {
			return nil, fmt.Errorf("voting user %d has a %d-byte key hash, want %d",
				i, len(fields.KeyHash), KeyHashLen)
		}

		group = append(group, VotingIdentity{
			KeyHash:  hex.EncodeToString(fields.KeyHash),
			CertHash: hex.EncodeToString(fields.CertHash),
		})
	}

	if len(group) == 0 {
		// The validator rejects an empty multisig requirement outright, so an
		// empty group is not a committee that cannot vote — it is a datum we
		// have misread.
		return nil, fmt.Errorf("hot datum names no voting users")
	}
	return group, nil
}
