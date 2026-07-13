package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Awen-online/cella/internal/cardano"
)

// A Cella instance belongs to somebody. It might be a consortium of delegates
// sharing one Constitutional Committee seat, or it might be a single member who
// holds the seat alone — both are common, and Cella should not assume the first.
//
// So the body is configuration, not code: a deployment points CELLA_ROSTER at a
// JSON file describing itself. Cella ships a placeholder so the chamber can be
// demonstrated, but a placeholder cannot authenticate anyone — its addresses are
// not real, so no wallet will ever match one.

// Member is one delegate in a constitutional body.
type Member struct {
	Name string `json:"name"` // display identity
	Role string `json:"role"` // portfolio / seat

	// Handle is how the member is known publicly — an X handle, usually. Shown
	// on the dashboard so the body is a group of people rather than a list of
	// strings.
	Handle string `json:"handle,omitempty"`

	// HandleURL is where Handle points. Derived from an @handle when omitted.
	HandleURL string `json:"handleUrl,omitempty"`

	// Address is the delegate's Cardano wallet address (bech32). It is how Cella
	// recognises them at sign-in: the public key inside their signature must hash
	// to a credential this address carries.
	Address string `json:"address"`

	// VoteKeyHash is the delegate's on-chain voting credential (hex,
	// blake2b-224) — the key hash that appears in the hot NFT datum's voting
	// group and that signs the committee's vote transaction. It is not
	// necessarily the same key as Address: the wallet gets them into Cella, the
	// voting key gets the committee's vote onto the chain.
	VoteKeyHash string `json:"voteKeyHash,omitempty"`
}

// Link is where the body can be found.
func (m Member) Link() string {
	if m.HandleURL != "" {
		return m.HandleURL
	}
	if h := strings.TrimPrefix(m.Handle, "@"); h != "" {
		return "https://x.com/" + h
	}
	return ""
}

// Body is the committee member — a consortium of delegates, or one member alone.
type Body struct {
	Name  string `json:"name"`            // "Cardano Curia"
	Short string `json:"short,omitempty"` // "The Curia"
	Kind  string `json:"kind"`            // "Constitutional Committee member"
	Blurb string `json:"blurb"`

	// Logo is a same-origin path Cella serves. It cannot be an external URL:
	// the Content-Security-Policy is img-src 'self', deliberately, and a
	// governance tool should not be loading its own identity from someone else's
	// server. Point CELLA_LOGO at a file on disk and Cella will serve it here.
	Logo string `json:"logo,omitempty"`

	Website string `json:"website,omitempty"`
	X       string `json:"x,omitempty"` // the body's own X profile

	Members []Member `json:"members"`
}

// Solo reports whether the body is a single member rather than a consortium.
// A one-person committee member is common, and "1 delegates" is not a thing.
func (b Body) Solo() bool { return len(b.Members) == 1 }

// Display is the shortest name that still identifies the body.
func (b Body) Display() string {
	if b.Short != "" {
		return b.Short
	}
	return b.Name
}

// LoadBody reads a body from a JSON file. An empty path yields the placeholder.
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
		return Body{}, fmt.Errorf("roster %s lists no members", path)
	}

	// Fail loudly at startup rather than silently at sign-in: a member whose
	// address does not decode can never be recognised, and discovering that when
	// they try to vote is far too late.
	for i, m := range body.Members {
		if m.Name == "" {
			return Body{}, fmt.Errorf("roster %s: member %d has no name", path, i)
		}
		if m.Address == "" {
			continue // a member may be listed before they register a wallet
		}
		if _, err := cardano.Credentials(m.Address); err != nil {
			return Body{}, fmt.Errorf("roster %s: %s has an unusable address: %w", path, m.Name, err)
		}
	}
	return body, nil
}

// ByCredential finds the member whose registered address carries the given key
// hash. This is the whole of Cella's wallet identity check: the credential comes
// from hashing the public key inside a verified signature, so a match proves the
// signer holds a key the member registered — not merely that they claimed to be
// that member.
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

// ByName finds a member by display name.
func (b Body) ByName(name string) (Member, bool) {
	for _, m := range b.Members {
		if m.Name == name {
			return m, true
		}
	}
	return Member{}, false
}

// demoBody is the roster used when no CELLA_ROSTER is configured: Cardano Curia,
// the consortium Cella is built for.
//
// The wallet addresses are deliberately absent. Nobody here can sign in until a
// real roster registers their address, so an unconfigured deployment cannot
// authenticate anybody by accident.
var demoBody = Body{
	Name:    "Cardano Curia",
	Short:   "The Curia",
	Kind:    "Constitutional Committee member",
	Blurb:   "A consortium that deliberates on Cardano governance actions, assesses their constitutionality, and casts a single committee vote with a shared rationale.",
	Logo:    "/brand/logo",
	Website: "https://www.cardanocuria.com",
	X:       "https://x.com/cardanocuria",
	Members: []Member{
		{Name: "James Meidinger", Role: "Member", Handle: "@blockjock65"},
		{Name: "Mladen Lamesevic", Role: "Member", Handle: "@MladenLm"},
		{Name: "Sheldon Hunt", Role: "Member", Handle: "@SundialSheldon"},
		{Name: "Ian McCullough", Role: "Member", Handle: "@CullahMusic"},
		{Name: "Kris Kowalsky", Role: "Member", Handle: "@KrisKowalsky"},
	},
}
