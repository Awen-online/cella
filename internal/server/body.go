package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	// Logo is a filename, resolved relative to this config file — the mark sits
	// next to the JSON that describes the body. Cella reads it and serves it
	// itself; it is never an external URL. The Content-Security-Policy is
	// img-src 'self' deliberately, because a page that can load images from
	// anywhere leaks which governance actions a committee is reading to whoever
	// hosts them — and a self-hostable tool should not go dark because someone
	// else's server did.
	Logo string `json:"logo,omitempty"`

	// LogoData is the mark itself, read from Logo at load. Not configuration.
	LogoData []byte `json:"-"`
	LogoMIME string `json:"-"`

	Website string `json:"website,omitempty"`
	X       string `json:"x,omitempty"` // the body's own X profile

	Members []Member `json:"members"`
}

// HasLogo reports whether the body brought its own mark.
func (b Body) HasLogo() bool { return len(b.LogoData) > 0 }

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

// ParseBody reads a body from JSON. It does not load the logo, which needs to
// be resolved against wherever the JSON came from.
func ParseBody(data []byte, source string) (Body, error) {
	var body Body
	if err := json.Unmarshal(data, &body); err != nil {
		return Body{}, fmt.Errorf("parse %s: %w", source, err)
	}
	if body.Name == "" {
		return Body{}, fmt.Errorf("%s: the body has no name", source)
	}
	if len(body.Members) == 0 {
		return Body{}, fmt.Errorf("%s lists no members", source)
	}

	// Fail loudly at startup rather than silently at sign-in: a member whose
	// address does not decode can never be recognised, and discovering that when
	// they try to vote is far too late.
	for i, m := range body.Members {
		if m.Name == "" {
			return Body{}, fmt.Errorf("%s: member %d has no name", source, i)
		}
		if m.Address == "" {
			continue // a member may be listed before they register a wallet
		}
		if _, err := cardano.Credentials(m.Address); err != nil {
			return Body{}, fmt.Errorf("%s: %s has an unusable address: %w", source, m.Name, err)
		}
	}
	return body, nil
}

// LoadBody reads a body from a JSON file, and its logo from alongside it.
func LoadBody(path string) (Body, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Body{}, fmt.Errorf("read body: %w", err)
	}

	body, err := ParseBody(data, path)
	if err != nil {
		return Body{}, err
	}

	// The mark sits next to the config that names it.
	if body.Logo != "" {
		logo := body.Logo
		if !filepath.IsAbs(logo) {
			logo = filepath.Join(filepath.Dir(path), logo)
		}
		raw, err := os.ReadFile(logo)
		if err != nil {
			return Body{}, fmt.Errorf("%s names a logo it does not have: %w", path, err)
		}
		body.SetLogo(raw, MIMEFor(logo))
	}
	return body, nil
}

// SetLogo attaches the body's mark.
func (b *Body) SetLogo(data []byte, mime string) {
	b.LogoData, b.LogoMIME = data, mime
	if len(data) > 0 {
		b.Logo = "/brand/logo" // the path the page will ask Cella for
	}
}

// MIMEFor guesses an image type from its extension. A logo is one of a handful
// of things and none of them need content sniffing.
func MIMEFor(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
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

// placeholderBody is what Cella falls back to when no body is configured. It is
// deliberately nobody: a real deployment must say who it is, and an instance
// that quietly impersonates a consortium it is not would be worse than one that
// admits it has not been told.
var placeholderBody = Body{
	Name:  "An unconfigured body",
	Kind:  "Constitutional Committee member",
	Blurb: "No body is configured. Point CELLA_BODY at a body.json — see body/curia.json for a worked example — so this chamber knows whose it is.",
	Members: []Member{
		{Name: "No members configured", Role: "Set CELLA_BODY"},
	},
}
