// Package txbody reads a Conway vote transaction: what it votes, on what, with
// which rationale, and who must sign it.
//
// This exists so that a delegate is never asked to sign something they cannot
// read. Witnessing a transaction means signing the blake2b-256 of its body —
// 1,387 bytes of CBOR, in the real Curia vote this package is tested against.
// Nobody can eyeball that. So before a witness round opens, Cella decodes the
// body and checks it against what the chamber actually decided: the right
// action, the right vote, the rationale the committee itself authored, and
// signers who are in the voting group the chain recognises. If any of those
// disagree, the round does not open.
//
// The point is not that Cella is trustworthy. The point is that Cella checks,
// and that a delegate can check Cella — the body hash this package computes is
// the same one `cardano-cli` prints, and the decode is the same one
// `cardano-cli debug transaction view` gives.
//
// Cella never builds a transaction and never submits one. It reads them.
package txbody

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

// Conway transaction body map keys. Verified against a real Constitutional
// Committee vote on mainnet (tx af5e7866…, cast by Cardano Curia).
const (
	keyInputs          = 0
	keyRequiredSigners = 14
	keyVotingProcs     = 19
)

// Voter roles, as the ledger tags them. A consortium's committee credential is
// a script, not a key — which is the case that matters here. The others are
// listed because a transaction may carry DRep or SPO votes too, and Cella must
// not mistake one of those for the committee's own.
const (
	voterCommitteeKey    = 0
	voterCommitteeScript = 1
	voterDRepKey         = 2
	voterDRepScript      = 3
	voterPool            = 4
)

// IsCommittee reports whether a vote was cast by the Constitutional Committee,
// as opposed to a DRep or a stake pool.
func (v Vote) IsCommittee() bool {
	return v.voterKind == voterCommitteeKey || v.voterKind == voterCommitteeScript
}

// Vote is how the ledger encodes a decision.
const (
	voteNo      = 0
	voteYes     = 1
	voteAbstain = 2
)

// Tx is a decoded transaction: the parts a committee needs to check before
// anyone signs it.
type Tx struct {
	// BodyHash is the blake2b-256 of the body's exact bytes. It is the
	// transaction's id, and it is what a witness signature is over. Everything
	// in this package exists to let a delegate know what this hash means.
	BodyHash string

	// Votes are the committee's votes carried by this transaction. There may be
	// several: a committee commonly votes on a batch of actions in one go, and
	// witnessing the transaction witnesses all of them.
	Votes []Vote

	// RequiredSigners are the key hashes the transaction demands witnesses from
	// — the delegates whose signatures the hot NFT script will check.
	RequiredSigners []string

	// Witnesses are the signatures already attached. A body circulating for
	// witnessing usually has none.
	Witnesses []Witness
}

// Vote is one committee vote inside a transaction.
type Vote struct {
	// VoterCredential is the committee's hot credential — for a consortium, the
	// hash of its hot NFT script.
	VoterCredential string
	VoterIsScript   bool

	// The governance action being voted on.
	ActionTxID  string
	ActionIndex int

	// Decision is Yes, No or Abstain.
	Decision string

	voterKind int

	// The rationale anchor: where the CIP-136 document lives, and the hash the
	// chain commits to. Cella authored a rationale and computed a hash of its
	// own; if these do not match, the transaction is anchoring something else.
	AnchorURL  string
	AnchorHash string
}

// Witness is one signature over the transaction body.
type Witness struct {
	PubKey    string // hex, 32 bytes
	Signature string // hex, 64 bytes
}

// KeyHash is the credential this witness's key hashes to — blake2b-224 of the
// public key. It is how a witness is matched to the voting group.
func (w Witness) KeyHash() string {
	pub, err := hex.DecodeString(w.PubKey)
	if err != nil {
		return ""
	}
	h, _ := blake2b.New(28, nil)
	h.Write(pub)
	return hex.EncodeToString(h.Sum(nil))
}

// Decode reads a signed or unsigned Conway transaction.
//
// The body's bytes are sliced out of the original CBOR rather than re-encoded,
// because the hash must be over exactly what the chain will see. Re-serialising
// a decoded structure risks producing different bytes — a different map order, a
// definite length where the original was indefinite — and therefore a different
// hash, which would be a different transaction.
func Decode(raw []byte) (Tx, error) {
	var tx Tx

	// A transaction is [ body, witness_set, is_valid, auxiliary_data ].
	var parts []cbor.RawMessage
	if err := cbor.Unmarshal(raw, &parts); err != nil {
		return tx, fmt.Errorf("not a transaction: %w", err)
	}
	if len(parts) < 2 {
		return tx, fmt.Errorf("transaction has %d parts, want at least 2 (body, witness set)", len(parts))
	}

	bodyBytes := []byte(parts[0])
	sum := blake2b.Sum256(bodyBytes)
	tx.BodyHash = hex.EncodeToString(sum[:])

	body := map[int]cbor.RawMessage{}
	if err := cbor.Unmarshal(bodyBytes, &body); err != nil {
		return tx, fmt.Errorf("transaction body is not a map: %w", err)
	}
	if _, ok := body[keyInputs]; !ok {
		return tx, fmt.Errorf("transaction body has no inputs; this is not a transaction body")
	}

	var err error
	if tx.RequiredSigners, err = decodeSigners(body[keyRequiredSigners]); err != nil {
		return tx, err
	}
	if tx.Votes, err = decodeVotes(body[keyVotingProcs]); err != nil {
		return tx, err
	}
	if tx.Witnesses, err = decodeWitnesses(parts[1]); err != nil {
		return tx, err
	}
	return tx, nil
}

// decodeSigners reads the required_signers set: 28-byte key hashes.
func decodeSigners(raw cbor.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil // a transaction may require no signers
	}
	var hashes [][]byte
	if err := cbor.Unmarshal(raw, &hashes); err != nil {
		return nil, fmt.Errorf("required signers: %w", err)
	}
	out := make([]string, 0, len(hashes))
	for i, h := range hashes {
		if len(h) != 28 {
			return nil, fmt.Errorf("required signer %d is %d bytes, want 28 (a blake2b-224 key hash)", i, len(h))
		}
		out = append(out, hex.EncodeToString(h))
	}
	return out, nil
}

// pair is one entry of a CBOR map, left undecoded.
type pair struct{ key, val cbor.RawMessage }

// mapPairs splits a CBOR map into its entries without decoding them.
//
// The voting-procedures map is keyed by arrays — a voter is [kind, credential],
// a governance action id is [txId, index] — and a Go map cannot be keyed by a
// slice. So the map is walked rather than unmarshalled.
func mapPairs(raw []byte) ([]pair, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty map")
	}

	b := raw[0]
	if b>>5 != 5 { // major type 5 is a map
		return nil, fmt.Errorf("expected a CBOR map, got major type %d", b>>5)
	}

	// Definite-length maps state their size in the header; an indefinite one runs
	// until a break. Both are legal, and the ledger has used both.
	var n int
	var off int
	switch info := b & 0x1f; {
	case info < 24:
		n, off = int(info), 1
	case info == 24:
		n, off = int(raw[1]), 2
	case info == 25:
		n, off = int(raw[1])<<8|int(raw[2]), 3
	case info == 31:
		n, off = -1, 1 // indefinite
	default:
		return nil, fmt.Errorf("map length encoding 0x%02x is not supported here", b)
	}

	dec := cbor.NewDecoder(bytes.NewReader(raw[off:]))
	var out []pair
	for i := 0; n < 0 || i < n; i++ {
		var k cbor.RawMessage
		if err := dec.Decode(&k); err != nil {
			if n < 0 && errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("map key %d: %w", i, err)
		}
		if n < 0 && len(k) == 1 && k[0] == 0xff { // the break that ends an indefinite map
			break
		}
		var v cbor.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, fmt.Errorf("map value %d: %w", i, err)
		}
		out = append(out, pair{k, v})
	}
	return out, nil
}

// decodeVotes reads the voting procedures:
//
//	{ voter => { govActionId => votingProcedure } }
//	voter            = [ kind, credential ]
//	govActionId      = [ txId, index ]
//	votingProcedure  = [ vote, anchor / nil ]
//	anchor           = [ url, dataHash ]
func decodeVotes(raw cbor.RawMessage) ([]Vote, error) {
	if len(raw) == 0 {
		return nil, nil // not a voting transaction
	}

	procs, err := mapPairs(raw)
	if err != nil {
		return nil, fmt.Errorf("voting procedures: %w", err)
	}

	var out []Vote
	for _, p := range procs {
		voterRaw, actionsRaw := p.key, p.val

		var voter struct {
			_          struct{} `cbor:",toarray"`
			Kind       int
			Credential []byte
		}
		if err := cbor.Unmarshal(voterRaw, &voter); err != nil {
			return nil, fmt.Errorf("voter: %w", err)
		}

		actions, err := mapPairs(actionsRaw)
		if err != nil {
			return nil, fmt.Errorf("votes for voter %x: %w", voter.Credential, err)
		}

		for _, ap := range actions {
			gaRaw, procRaw := ap.key, ap.val
			var ga struct {
				_     struct{} `cbor:",toarray"`
				TxID  []byte
				Index int
			}
			if err := cbor.Unmarshal(gaRaw, &ga); err != nil {
				return nil, fmt.Errorf("governance action id: %w", err)
			}

			var proc struct {
				_      struct{} `cbor:",toarray"`
				Vote   int
				Anchor cbor.RawMessage
			}
			if err := cbor.Unmarshal(procRaw, &proc); err != nil {
				return nil, fmt.Errorf("voting procedure: %w", err)
			}

			decision, err := decisionOf(proc.Vote)
			if err != nil {
				return nil, err
			}

			v := Vote{
				voterKind:       voter.Kind,
				VoterCredential: hex.EncodeToString(voter.Credential),
				VoterIsScript:   voter.Kind == voterCommitteeScript || voter.Kind == voterDRepScript,
				ActionTxID:      hex.EncodeToString(ga.TxID),
				ActionIndex:     ga.Index,
				Decision:        decision,
			}

			// The anchor is optional in the ledger, but a committee voting
			// without a rationale is a committee refusing to say why.
			if len(proc.Anchor) > 0 && string(proc.Anchor) != "\xf6" { // f6 = null
				var anchor struct {
					_    struct{} `cbor:",toarray"`
					URL  string
					Hash []byte
				}
				if err := cbor.Unmarshal(proc.Anchor, &anchor); err != nil {
					return nil, fmt.Errorf("anchor: %w", err)
				}
				v.AnchorURL = anchor.URL
				v.AnchorHash = hex.EncodeToString(anchor.Hash)
			}
			out = append(out, v)
		}
	}
	return out, nil
}

// decodeWitnesses reads the vkey witnesses from the witness set (key 0).
func decodeWitnesses(raw cbor.RawMessage) ([]Witness, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	set := map[int]cbor.RawMessage{}
	if err := cbor.Unmarshal(raw, &set); err != nil {
		// An unsigned body may carry an empty or absent witness set.
		return nil, nil
	}
	vkeys, ok := set[0]
	if !ok {
		return nil, nil
	}

	var raws []struct {
		_         struct{} `cbor:",toarray"`
		PubKey    []byte
		Signature []byte
	}
	if err := cbor.Unmarshal(vkeys, &raws); err != nil {
		return nil, fmt.Errorf("vkey witnesses: %w", err)
	}

	out := make([]Witness, 0, len(raws))
	for _, w := range raws {
		out = append(out, Witness{
			PubKey:    hex.EncodeToString(w.PubKey),
			Signature: hex.EncodeToString(w.Signature),
		})
	}
	return out, nil
}

func decisionOf(v int) (string, error) {
	switch v {
	case voteNo:
		return "No", nil
	case voteYes:
		return "Yes", nil
	case voteAbstain:
		return "Abstain", nil
	default:
		return "", fmt.Errorf("unknown vote %d; the ledger encodes No=0, Yes=1, Abstain=2", v)
	}
}
