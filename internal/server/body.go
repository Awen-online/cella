package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Awen-online/cella/internal/cardano"
)

// The body is the consortium of delegates who deliberate and, between them,
// hold one Constitutional Committee seat.
//
// It is configuration, not code: a deployment points CELLA_ROSTER at a JSON
// file describing its own delegates. Cella falls back to a placeholder roster
// so the chamber can be demonstrated, but a placeholder roster cannot
// authenticate anyone — its addresses are not real, so no wallet will ever
// match one.

// Member is one delegate in a constitutional body.
type Member struct {
	Name string `json:"name"` // display identity
	Role string `json:"role"` // portfolio / seat

	// Address is the delegate's Cardano wallet address (bech32). It is how
	// Cella recognises them at sign-in: the public key inside their signature
	// must hash to a credential this address carries.
	Address string `json:"address"`

	// VoteKeyHash is the delegate's on-chain voting credential (hex,
	// blake2b-224) — the key hash that appears in the hot NFT datum's voting
	// group and that signs the committee's vote transaction. It is not
	// necessarily the same key as Address: the wallet gets them into Cella, the
	// voting key gets the committee's vote onto the chain.
	VoteKeyHash string `json:"voteKeyHash,omitempty"`
}

// Body is a constitutional committee / delegate consortium.
type Body struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Blurb   string   `json:"blurb"`
	Members []Member `json:"members"`
}

// LoadBody reads a roster from a JSON file. An empty path yields the
// placeholder roster.
func LoadBody(path string) (Body, error) {
	if path == "" {
		return demoBody, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Body{}, fmt.Errorf("read roster: %w", err)
	}

	var body Body
	if err := json.Unmarshal(b, &body); err != nil {
		return Body{}, fmt.Errorf("parse roster %s: %w", path, err)
	}
	if len(body.Members) == 0 {
		return Body{}, fmt.Errorf("roster %s lists no delegates", path)
	}

	// Fail loudly at startup rather than silently at sign-in: a delegate whose
	// address does not decode can never be recognised, and discovering that
	// when they try to vote is far too late.
	for i, m := range body.Members {
		if m.Name == "" {
			return Body{}, fmt.Errorf("roster %s: delegate %d has no name", path, i)
		}
		if m.Address == "" {
			continue // a delegate may be listed before they register a wallet
		}
		if _, err := cardano.Credentials(m.Address); err != nil {
			return Body{}, fmt.Errorf("roster %s: %s has an unusable address: %w", path, m.Name, err)
		}
	}
	return body, nil
}

// ByCredential finds the delegate whose registered address carries the given
// key hash. This is the whole of Cella's wallet identity check: the credential
// comes from hashing the public key inside a verified signature, so a match
// proves the signer holds a key the delegate registered — not merely that they
// claimed to be that delegate.
func (b Body) ByCredential(keyHash string) (Member, bool) {
	keyHash = strings.ToLower(keyHash)
	for _, m := range b.Members {
		if m.Address == "" {
			continue
		}
		creds, err := cardano.Credentials(m.Address)
		if err != nil {
			continue // validated at load; an undecodable address matches nobody
		}
		for _, c := range creds {
			if c == keyHash {
				return m, true
			}
		}
	}
	return Member{}, false
}

// ByName finds a delegate by display name.
func (b Body) ByName(name string) (Member, bool) {
	for _, m := range b.Members {
		if m.Name == name {
			return m, true
		}
	}
	return Member{}, false
}

// demoBody is the placeholder roster used when no CELLA_ROSTER is configured.
// The addresses are illustrative and deliberately do not decode as real bech32,
// so they cannot accidentally authenticate anyone.
var demoBody = Body{
	Name:  "Cardano Curia",
	Kind:  "Constitutional Committee member",
	Blurb: "A consortium that deliberates on Cardano governance actions, assesses their constitutionality, and casts a single committee vote with a shared rationale.",
	Members: []Member{
		{Name: "Faustina Vela", Role: "Delegate · Treasury & Withdrawals", Address: ""},
		{Name: "Cassius Aurel", Role: "Delegate · Protocol Parameters", Address: ""},
		{Name: "Junia Marcia", Role: "Delegate · Constitution & Precedent", Address: ""},
		{Name: "Titus Varo", Role: "Delegate · Community & Outreach", Address: ""},
		{Name: "Cullah", Role: "Delegate · At-large", Address: "addr1q8ejkg9t0tkqxkms3nqe2e90rgdn680mg9vfq5ygd06j94d0xdpt8x0tr7mpserrhsssmh0d8ug7494ndwr53rcs3veq6dulc9"},
	},
}
