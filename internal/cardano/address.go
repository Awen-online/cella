// Package cardano understands just enough of Cardano's address format (CIP-19)
// to answer one question: does this signature come from this delegate?
//
// The answer is never taken from what the browser claims. A CIP-30 wallet sends
// an address alongside its signature, but a caller can send any address they
// like. What cannot be faked is the signature itself: the Ed25519 public key
// inside it hashes to a credential, and that credential either appears in the
// delegate's registered address or it does not.
package cardano

import (
	"encoding/hex"
	"fmt"

	"github.com/Awen-online/cella/internal/bech32"
	"golang.org/x/crypto/blake2b"
)

// KeyHashLen is the length of a Cardano credential: blake2b-224 is 28 bytes.
const KeyHashLen = 28

// KeyHash is the blake2b-224 of an Ed25519 public key — the credential form in
// which Cardano records who someone is.
func KeyHash(pub []byte) []byte {
	h, _ := blake2b.New(KeyHashLen, nil)
	h.Write(pub)
	return h.Sum(nil)
}

// Credentials returns every key hash embedded in a bech32 Cardano address, hex
// encoded.
//
// An address can carry more than one. A base address (addr1…) holds both a
// payment credential and a stake credential, and a CIP-30 wallet may sign with
// either depending on which address the page asked it to use — Cella asks for
// the reward address first and falls back to a used address, so both are
// legitimate proofs of the same delegate. A reward address (stake1…) or an
// enterprise address holds only one.
//
// Address layout (CIP-19): one header byte, then the payment credential, then
// for a base address the stake credential.
func Credentials(addr string) ([]string, error) {
	_, raw, err := bech32.Decode(addr)
	if err != nil {
		return nil, err
	}
	if len(raw) < 1+KeyHashLen {
		return nil, fmt.Errorf("address is too short to carry a credential (%d bytes)", len(raw))
	}

	// The first credential sits right after the header, whatever the address
	// type: the payment credential for a payment address, the stake credential
	// for a reward address.
	out := []string{hex.EncodeToString(raw[1 : 1+KeyHashLen])}

	// A base address carries the stake credential too.
	if len(raw) >= 1+2*KeyHashLen {
		out = append(out, hex.EncodeToString(raw[1+KeyHashLen:1+2*KeyHashLen]))
	}
	return out, nil
}
