package txbody

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// The fixture is Cardano Curia's own Constitutional Committee vote on mainnet,
// pulled from the chain — not a shape anyone invented. If this decoder is wrong,
// Cella misreports what a delegate is about to sign, which is the worst thing it
// could do. So it is tested against the real thing.
//
//	tx af5e78661826082c673ea40fcd26768014be58d48702c382fd6c0e302330e79e
const (
	curiaVoteTx    = "af5e78661826082c673ea40fcd26768014be58d48702c382fd6c0e302330e79e"
	curiaHotScript = "84feba943c574d25984175cf8257959e6b3a1c64143d85e64fef6bd5"
)

func realVote(t *testing.T) []byte {
	t.Helper()
	h, err := os.ReadFile("testdata/curia-vote-tx.hex")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(h)))
	if err != nil {
		t.Fatalf("fixture is not hex: %v", err)
	}
	return raw
}

// The single most important assertion in this package.
//
// A witness signs the blake2b-256 of the transaction body. If Cella computes
// that hash differently from the ledger, every check downstream is checking the
// wrong thing — and a delegate would be told they are signing one transaction
// while signing another.
//
// The body's bytes must be sliced out of the original CBOR, never re-encoded: a
// re-serialised body can differ (map order, definite vs indefinite length) and
// therefore hash differently, which would be a different transaction.
func TestBodyHashIsTheTransactionID(t *testing.T) {
	tx, err := Decode(realVote(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if tx.BodyHash != curiaVoteTx {
		t.Fatalf("body hash = %s\nwant       %s\n\nthis is the hash a witness signs; if it is wrong, nothing else here means anything",
			tx.BodyHash, curiaVoteTx)
	}
}

// What the transaction actually votes. This is what Cella must show a delegate,
// and what it must check against the chamber's own decision.
func TestDecodesTheCommitteesVotes(t *testing.T) {
	tx, err := Decode(realVote(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// A committee commonly votes on a batch in one transaction — witnessing it
	// witnesses all of them, which is exactly why a delegate must be shown all
	// of them.
	if len(tx.Votes) != 5 {
		t.Fatalf("got %d votes, want 5 — this transaction carries a batch", len(tx.Votes))
	}

	for _, v := range tx.Votes {
		if v.VoterCredential != curiaHotScript {
			t.Errorf("voter = %s, want the Curia's hot script %s", v.VoterCredential, curiaHotScript)
		}
		if !v.VoterIsScript {
			t.Error("the voter should be a script credential: a consortium votes through the hot NFT script")
		}
		if v.Decision != "Yes" {
			t.Errorf("decision = %q, want Yes", v.Decision)
		}
		// A committee voting without a rationale is a committee refusing to say
		// why. Every vote here anchors one.
		if !strings.HasPrefix(v.AnchorURL, "ipfs://") {
			t.Errorf("anchor URL = %q, want an ipfs:// rationale", v.AnchorURL)
		}
		if len(v.AnchorHash) != 64 {
			t.Errorf("anchor hash = %q, want 32 bytes (blake2b-256 of the CIP-136 document)", v.AnchorHash)
		}
		if v.ActionIndex != 0 || len(v.ActionTxID) != 64 {
			t.Errorf("governance action = %s#%d, malformed", v.ActionTxID, v.ActionIndex)
		}
	}
}

// The transaction names the delegates whose signatures the hot NFT script will
// demand. Cella checks every uploaded witness against this list — and against
// the voting group in the datum.
func TestDecodesRequiredSigners(t *testing.T) {
	tx, err := Decode(realVote(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// The Curia's five members need ceil(5/2) = 3 signatures. The transaction
	// carries exactly three required signers — the quorum rule, confirmed on a
	// real vote rather than only in the validator's source.
	if len(tx.RequiredSigners) != 3 {
		t.Fatalf("got %d required signers, want 3 (ceil(5/2) — the quorum)", len(tx.RequiredSigners))
	}
	want := map[string]bool{
		"3afbd682fb0cf214ddbbc10e7f549654c6d21aca0e43322365b501ed": true,
		"70cd4376d16333747880d18f226a1c553a88810e2314ab5ebc6b5907": true,
		"36134c554808d1651e7f6157fef4bae513dbd6c327dd8a4261a11687": true,
	}
	for _, s := range tx.RequiredSigners {
		if !want[s] {
			t.Errorf("unexpected required signer %s", s)
		}
		if len(s) != 56 {
			t.Errorf("required signer %s is not a 28-byte key hash", s)
		}
	}
}

// Each delegate's witness contributes a public key and a signature. Cella
// identifies who signed by hashing the key — the same way the ledger does.
func TestWitnessesMatchTheRequiredSigners(t *testing.T) {
	tx, err := Decode(realVote(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	// Four witnesses: the three voting delegates, plus the payment key that paid
	// the fee. The payment key is not a required signer, and must not be counted
	// towards quorum.
	if len(tx.Witnesses) != 4 {
		t.Fatalf("got %d witnesses, want 4 (3 voters + the payment key)", len(tx.Witnesses))
	}

	required := map[string]bool{}
	for _, s := range tx.RequiredSigners {
		required[s] = true
	}

	var voters, others int
	for _, w := range tx.Witnesses {
		kh := w.KeyHash()
		if len(kh) != 56 {
			t.Fatalf("witness key hash %q is malformed", kh)
		}
		if required[kh] {
			voters++
		} else {
			others++
		}
		if len(w.PubKey) != 64 || len(w.Signature) != 128 {
			t.Errorf("witness has a %d-char key and %d-char signature; want 64 and 128",
				len(w.PubKey), len(w.Signature))
		}
	}
	if voters != 3 {
		t.Errorf("%d witnesses matched a required signer, want 3 — quorum would be miscounted", voters)
	}
	if others != 1 {
		t.Errorf("%d witnesses matched nothing, want 1 (the payment key)", others)
	}
}

// A key hash is blake2b-224 of the public key. Getting the digest size wrong
// would make every witness look like a stranger.
func TestKeyHashIsBlake2b224(t *testing.T) {
	tx, _ := Decode(realVote(t))
	for _, w := range tx.Witnesses {
		if got := len(w.KeyHash()); got != 56 {
			t.Fatalf("key hash is %d hex chars, want 56 (28 bytes)", got)
		}
	}
	// A malformed key must not produce a plausible-looking hash.
	if got := (Witness{PubKey: "zzz"}).KeyHash(); got != "" {
		t.Errorf("a non-hex public key produced a key hash: %q", got)
	}
}

func TestDecodeRejectsRubbish(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"not CBOR":            "6e6f7420636276f0",
		"a bare map":          "a10001",
		"an array of one":     "8100",
		"body with no inputs": "8283a10201a0f5f6", // a "body" that has no input set
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			raw, _ := hex.DecodeString(h)
			if tx, err := Decode(raw); err == nil {
				t.Errorf("Decode succeeded on %s: %+v", name, tx)
			}
		})
	}
}

// The ledger encodes No=0, Yes=1, Abstain=2. Confusing Yes and No is the single
// worst mistake this package could make, so the mapping is pinned rather than
// left to a comment.
func TestVoteEncoding(t *testing.T) {
	for v, want := range map[int]string{0: "No", 1: "Yes", 2: "Abstain"} {
		got, err := decisionOf(v)
		if err != nil || got != want {
			t.Errorf("decisionOf(%d) = %q, %v; want %q", v, got, err, want)
		}
	}
	if _, err := decisionOf(3); err == nil {
		t.Error("an unknown vote code was accepted; better to fail than to guess")
	}
}

// A transaction may carry DRep or SPO votes alongside. Cella must not mistake
// one of those for the committee's own decision.
func TestCommitteeVotesAreIdentified(t *testing.T) {
	tx, err := Decode(realVote(t))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	for _, v := range tx.Votes {
		if !v.IsCommittee() {
			t.Errorf("a Constitutional Committee vote was not recognised as one: %+v", v)
		}
	}
}
