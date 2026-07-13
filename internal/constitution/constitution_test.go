package constitution

import (
	"strings"
	"testing"
)

// A Constitution with no anchors is a Constitution nobody can cite. Every
// article and section must be linkable, in every revision.
func TestEveryRevisionHasANavigableTOC(t *testing.T) {
	for _, v := range Versions {
		t.Run(v.Key, func(t *testing.T) {
			html, toc, got, err := HTML(v.Key)
			if err != nil {
				t.Fatalf("HTML(%s): %v", v.Key, err)
			}
			if got.Key != v.Key {
				t.Fatalf("resolved to %s, want %s", got.Key, v.Key)
			}
			if len(toc) == 0 {
				t.Fatal("the table of contents is empty")
			}

			for _, e := range toc {
				if e.ID == "" {
					t.Errorf("%q has no anchor", e.Text)
				}
				if e.Text == "" {
					t.Errorf("anchor #%s has no text", e.ID)
				}
				// Every entry must point at a heading that actually exists, or the
				// contents are a list of dead links.
				if !strings.Contains(string(html), `id="`+e.ID+`"`) {
					t.Errorf("table of contents links to #%s, which is not in the document", e.ID)
				}
			}
		})
	}
}

// The interim revision writes its articles as level-3 headings while the
// current one uses level-2. A table of contents that only gathered level-2
// headings would be empty on v0 — which is exactly the failure in the
// deployment this replaces.
func TestInterimRevisionArticlesAreListed(t *testing.T) {
	_, toc, _, err := HTML("v0")
	if err != nil {
		t.Fatalf("HTML(v0): %v", err)
	}

	var articles int
	for _, e := range toc {
		if strings.HasPrefix(e.Text, "ARTICLE ") {
			articles++
		}
	}
	if articles < 8 {
		t.Errorf("v0 lists %d articles in its contents, want at least 8 — level-3 headings were dropped", articles)
	}
}

// The action detail page deep-links into these anchors. If a heading is
// reworded and its slug changes, those links die silently — so assert them
// here, where the breakage is loud.
func TestAlignmentAnchorsExist(t *testing.T) {
	html, _, _, err := HTML("") // the current revision
	if err != nil {
		t.Fatalf("HTML: %v", err)
	}

	for _, id := range []string{
		"article-i-cardano-blockchain-tenets-and-guardrails",
		"article-ii-community-and-governance",
		"article-iii-constitutional-committee",
		"article-iv-amendment-process",
		"appendix-i-cardano-blockchain-guardrails",
	} {
		if !strings.Contains(string(html), `id="`+id+`"`) {
			t.Errorf("#%s is not in the current Constitution; the action page links to it", id)
		}
	}
}

// Section anchors must be unique even when a dozen articles each open with a
// "Section 1" — otherwise every one of them links to the first.
func TestDuplicateHeadingsGetDistinctAnchors(t *testing.T) {
	_, toc, _, err := HTML("v1") // v1 repeats "Section 1" under every article
	if err != nil {
		t.Fatalf("HTML(v1): %v", err)
	}

	seen := map[string]string{}
	for _, e := range toc {
		if prev, dup := seen[e.ID]; dup {
			t.Errorf("anchor #%s is used by both %q and %q", e.ID, prev, e.Text)
		}
		seen[e.ID] = e.Text
	}
}

func TestTextAndHTMLResolveTheSameRevision(t *testing.T) {
	for _, key := range []string{"", "v2.4", "v1", "v0", "nonsense"} {
		_, tv, err := Text(key)
		if err != nil {
			t.Fatalf("Text(%q): %v", key, err)
		}
		_, _, hv, err := HTML(key)
		if err != nil {
			t.Fatalf("HTML(%q): %v", key, err)
		}
		if tv.Key != hv.Key {
			t.Errorf("Text(%q) gave %s but HTML(%q) gave %s — the review and the reader would disagree",
				key, tv.Key, key, hv.Key)
		}
	}

	// An unknown revision must fall back to the current one, not to nothing.
	_, _, v, _ := HTML("nonsense")
	if !v.Current {
		t.Errorf("unknown revision resolved to %s, want the current one", v.Key)
	}
}
