// Package rationale builds the Constitutional Committee's vote rationale as a
// CIP-136 JSON-LD document and computes its on-chain anchor hash.
//
// CIP-136 ("Governance Metadata — Constitutional Committee Vote Rationale")
// extends CIP-100. The document Cella emits here is the real artifact: the same
// bytes are what a committee anchors and what `cardano-cli hash anchor-data
// --file-text` hashes, so AnchorHash reproduces the value that goes on-chain
// alongside the vote.
//
// Spec: https://github.com/cardano-foundation/CIPs/tree/master/CIP-0136
package rationale

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/blake2b"
)

// SummaryLimit is the character ceiling CIP-136 places on the summary.
const SummaryLimit = 300

// context is the CIP-136 common JSON-LD context, reproduced verbatim from the
// CIP so the documents Cella emits resolve against the published vocabulary.
const context = `{
    "@language": "en-us",
    "CIP100": "https://github.com/cardano-foundation/CIPs/blob/master/CIP-0100/README.md#",
    "CIP136": "https://github.com/cardano-foundation/CIPs/blob/master/CIP-0136/README.md#",
    "hashAlgorithm": "CIP100:hashAlgorithm",
    "body": {
        "@id": "CIP136:body",
        "@context": {
            "references": {
                "@id": "CIP100:references",
                "@container": "@set",
                "@context": {
                    "GovernanceMetadata": "CIP100:GovernanceMetadataReference",
                    "Other": "CIP100:OtherReference",
                    "label": "CIP100:reference-label",
                    "uri": "CIP100:reference-uri",
                    "RelevantArticles": "CIP136:RelevantArticles"
                }
            },
            "summary": "CIP136:summary",
            "rationaleStatement": "CIP136:rationaleStatement",
            "precedentDiscussion": "CIP136:precedentDiscussion",
            "counterargumentDiscussion": "CIP136:counterargumentDiscussion",
            "conclusion": "CIP136:conclusion",
            "internalVote": {
                "@id": "CIP136:internalVote",
                "@context": {
                    "constitutional": "CIP136:constitutional",
                    "unconstitutional": "CIP136:unconstitutional",
                    "abstain": "CIP136:abstain",
                    "didNotVote": "CIP136:didNotVote",
                    "againstVote": "CIP136:againstVote"
                }
            }
        }
    },
    "authors": {
        "@id": "CIP100:authors",
        "@container": "@set",
        "@context": {
            "did": "@id",
            "name": "http://xmlns.com/foaf/0.1/name",
            "witness": {
                "@id": "CIP100:witness",
                "@context": {
                    "witnessAlgorithm": "CIP100:witnessAlgorithm",
                    "publicKey": "CIP100:publicKey",
                    "signature": "CIP100:signature"
                }
            }
        }
    }
}`

// Doc is a complete CIP-136 rationale document. Field order matches the CIP's
// own examples, and encoding/json preserves it.
type Doc struct {
	Context       json.RawMessage `json:"@context"`
	HashAlgorithm string          `json:"hashAlgorithm"`
	Body          Body            `json:"body"`
	Authors       []Author        `json:"authors"`
}

// Body carries the committee's reasoning. Summary and RationaleStatement are
// required by the CIP; the rest are optional and omitted when empty.
type Body struct {
	Summary                   string        `json:"summary"`
	RationaleStatement        string        `json:"rationaleStatement"`
	PrecedentDiscussion       string        `json:"precedentDiscussion,omitempty"`
	CounterargumentDiscussion string        `json:"counterargumentDiscussion,omitempty"`
	Conclusion                string        `json:"conclusion,omitempty"`
	InternalVote              *InternalVote `json:"internalVote,omitempty"`
	References                []Reference   `json:"references,omitempty"`
}

// InternalVote is how the body's own delegates split before the committee cast
// its single on-chain vote. It is the transparency the CIP asks a multi-member
// committee to publish, and it maps directly onto Cella's chamber: a delegate
// voting Yes judges the action constitutional, No unconstitutional.
type InternalVote struct {
	Constitutional   int `json:"constitutional"`
	Unconstitutional int `json:"unconstitutional"`
	Abstain          int `json:"abstain"`
	DidNotVote       int `json:"didNotVote"`
	AgainstVote      int `json:"againstVote,omitempty"`
}

// Reference is a citation. Type is "RelevantArticles" for a Constitution
// article, "GovernanceMetadata" for another governance document, else "Other".
type Reference struct {
	Type  string `json:"@type"`
	Label string `json:"label"`
	URI   string `json:"uri"`
}

// Author endorses the document. The CIP requires a witness object to be
// present; its fields are only populated once the author actually signs, so an
// unwitnessed document carries an empty witness.
type Author struct {
	Name    string  `json:"name"`
	Witness Witness `json:"witness"`
}

// Witness is an author's Ed25519 signature over the canonicalized body.
type Witness struct {
	WitnessAlgorithm string `json:"witnessAlgorithm,omitempty"`
	PublicKey        string `json:"publicKey,omitempty"`
	Signature        string `json:"signature,omitempty"`
}

// New assembles a CIP-136 document from the committee's reasoning, its internal
// vote split, the works it cites, and the delegates authoring it.
func New(body Body, authors []string) Doc {
	as := make([]Author, 0, len(authors))
	for _, name := range authors {
		as = append(as, Author{Name: name})
	}
	return Doc{
		Context:       json.RawMessage(context),
		HashAlgorithm: "blake2b-256",
		Body:          body,
		Authors:       as,
	}
}

// Validate reports what the CIP requires but the document is missing.
func (d Doc) Validate() error {
	var missing []string
	if strings.TrimSpace(d.Body.Summary) == "" {
		missing = append(missing, "a summary")
	}
	if strings.TrimSpace(d.Body.RationaleStatement) == "" {
		missing = append(missing, "a rationale statement")
	}
	if len(missing) > 0 {
		return fmt.Errorf("rationale needs %s", strings.Join(missing, " and "))
	}
	if n := utf8.RuneCountInString(d.Body.Summary); n > SummaryLimit {
		return fmt.Errorf("summary is %d characters; CIP-136 allows at most %d", n, SummaryLimit)
	}
	return nil
}

// JSONLD serializes the document as the .jsonld file a committee anchors.
// The bytes are what AnchorHash hashes, so they must not be reformatted
// afterwards — a single changed byte changes the anchor.
func (d Doc) JSONLD() ([]byte, error) {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// AnchorHash is the blake2b-256 of the document bytes, hex-encoded — the
// anchor hash submitted on-chain with the vote, and the same value
// `cardano-cli hash anchor-data --file-text` prints for these bytes.
func AnchorHash(jsonld []byte) string {
	sum := blake2b.Sum256(jsonld)
	return hex.EncodeToString(sum[:])
}
