package rationale

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The anchor hash is the one value in Cella that a third party will check
// against the chain, so pin it to the CIP's own published test vector: the
// blake2b-256 of CIP-136's example document must come out at the file hash the
// CIP publishes for it. If this fails, every anchor Cella produces is wrong.
func TestAnchorHashMatchesCIP136TestVector(t *testing.T) {
	const (
		file = "testdata/treasury-withdrawal-unconstitutional.jsonld"
		want = "7065bd1dcdde9c512f973519085ea55872fdf1a78eddb6907149dde1541e8044"
	)

	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read test vector: %v", err)
	}
	if got := AnchorHash(b); got != want {
		t.Errorf("AnchorHash(CIP-136 example) = %s, want %s\n"+
			"(if the byte count is off, the fixture's line endings were rewritten on checkout)", got, want)
	}
}

// The document we emit must satisfy the CIP's schema: hashAlgorithm, authors
// and body at the top level; summary and rationaleStatement inside the body.
func TestJSONLDShape(t *testing.T) {
	doc := New(Body{
		Summary:            "The committee finds the withdrawal constitutional.",
		RationaleStatement: "The request is proportionate and within the treasury guardrails.",
		Conclusion:         "Approved.",
		InternalVote:       &InternalVote{Constitutional: 3, Unconstitutional: 1, Abstain: 1, DidNotVote: 2},
		References: []Reference{
			{Type: "RelevantArticles", Label: "Article IV", URI: "https://example.org/constitution#art-4"},
		},
	}, []string{"Junia Marcia", "Cullah"})

	b, err := doc.JSONLD()
	if err != nil {
		t.Fatalf("JSONLD: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("emitted document is not valid JSON: %v", err)
	}
	for _, k := range []string{"@context", "hashAlgorithm", "body", "authors"} {
		if _, ok := got[k]; !ok {
			t.Errorf("document is missing the required top-level %q", k)
		}
	}
	if got["hashAlgorithm"] != "blake2b-256" {
		t.Errorf("hashAlgorithm = %v, want blake2b-256", got["hashAlgorithm"])
	}

	body, ok := got["body"].(map[string]any)
	if !ok {
		t.Fatal("body is not an object")
	}
	for _, k := range []string{"summary", "rationaleStatement", "internalVote", "references"} {
		if _, ok := body[k]; !ok {
			t.Errorf("body is missing %q", k)
		}
	}

	iv, ok := body["internalVote"].(map[string]any)
	if !ok {
		t.Fatal("internalVote is not an object")
	}
	if iv["constitutional"] != float64(3) || iv["unconstitutional"] != float64(1) ||
		iv["abstain"] != float64(1) || iv["didNotVote"] != float64(2) {
		t.Errorf("internalVote did not round-trip: %v", iv)
	}
	// againstVote is omitted when zero, but the other counts are meaningful at
	// zero and must always be present.
	if _, ok := iv["againstVote"]; ok {
		t.Error("againstVote should be omitted when zero")
	}

	// Every author carries a witness object even before anyone has signed.
	authors, ok := got["authors"].([]any)
	if !ok || len(authors) != 2 {
		t.Fatalf("authors = %v, want 2 entries", got["authors"])
	}
	for _, a := range authors {
		am := a.(map[string]any)
		if _, ok := am["witness"]; !ok {
			t.Errorf("author %v has no witness object", am["name"])
		}
	}
}

// An unset optional field must be absent from the body rather than present and
// empty — a blank precedentDiscussion asserts "no precedent was discussed",
// which is not the same as not having discussed it.
//
// Note this has to inspect the body object, not the raw bytes: the @context
// declares every one of these terms by name, so a substring search would find
// them in the vocabulary regardless.
func TestOptionalFieldsAreOmitted(t *testing.T) {
	doc := New(Body{Summary: "s", RationaleStatement: "r"}, nil)
	b, err := doc.JSONLD()
	if err != nil {
		t.Fatalf("JSONLD: %v", err)
	}
	var got struct {
		Body map[string]any `json:"body"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"precedentDiscussion", "counterargumentDiscussion", "conclusion", "internalVote", "references"} {
		if _, ok := got.Body[k]; ok {
			t.Errorf("empty %q was emitted in the body; want it omitted", k)
		}
	}
	// The two the CIP requires stay, even so.
	for _, k := range []string{"summary", "rationaleStatement"} {
		if _, ok := got.Body[k]; !ok {
			t.Errorf("required %q was omitted from the body", k)
		}
	}
}

// The same reasoning must always hash to the same anchor, or a committee could
// not reproduce the hash it submitted.
func TestJSONLDIsDeterministic(t *testing.T) {
	body := Body{
		Summary:            "Constitutional.",
		RationaleStatement: "Within the guardrails.",
		InternalVote:       &InternalVote{Constitutional: 2, Abstain: 1},
	}
	first, err := New(body, []string{"Cullah"}).JSONLD()
	if err != nil {
		t.Fatalf("JSONLD: %v", err)
	}
	for i := 0; i < 50; i++ {
		again, err := New(body, []string{"Cullah"}).JSONLD()
		if err != nil {
			t.Fatalf("JSONLD: %v", err)
		}
		if AnchorHash(again) != AnchorHash(first) {
			t.Fatalf("anchor hash is not stable across identical documents (iteration %d)", i)
		}
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		body    Body
		wantErr string
	}{
		{
			name: "complete",
			body: Body{Summary: "s", RationaleStatement: "r"},
		},
		{
			name:    "no summary",
			body:    Body{RationaleStatement: "r"},
			wantErr: "a summary",
		},
		{
			name:    "no statement",
			body:    Body{Summary: "s"},
			wantErr: "a rationale statement",
		},
		{
			name:    "whitespace is not content",
			body:    Body{Summary: "   ", RationaleStatement: "\n\t "},
			wantErr: "a summary and a rationale statement",
		},
		{
			name:    "summary over the CIP limit",
			body:    Body{Summary: strings.Repeat("x", SummaryLimit+1), RationaleStatement: "r"},
			wantErr: "at most 300",
		},
		{
			name: "summary exactly at the limit",
			body: Body{Summary: strings.Repeat("x", SummaryLimit), RationaleStatement: "r"},
		},
		{
			// The limit is characters, not bytes: 300 multi-byte runes are legal.
			name: "multi-byte summary at the limit",
			body: Body{Summary: strings.Repeat("é", SummaryLimit), RationaleStatement: "r"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := New(tc.body, nil).Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error mentioning %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() = %q, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}
