package server

import (
	"os"
	"path/filepath"
	"testing"
)

// A committee seat held by one person is as common as a consortium, and Cella
// must not describe it as "1 delegates".
func TestSoloBody(t *testing.T) {
	solo := Body{Name: "A. N. Other", Members: []Member{{Name: "A. N. Other"}}}
	if !solo.Solo() {
		t.Error("a one-member body did not report itself Solo()")
	}

	consortium := Body{Name: "Cardano Curia", Members: []Member{{Name: "One"}, {Name: "Two"}}}
	if consortium.Solo() {
		t.Error("a two-member body reported itself Solo()")
	}

	var empty Body
	if empty.Solo() {
		t.Error("an empty body reported itself Solo()")
	}
}

// The short name is what headings use; it falls back to the full name.
func TestBodyDisplay(t *testing.T) {
	if got := (Body{Name: "Cardano Curia", Short: "The Curia"}).Display(); got != "The Curia" {
		t.Errorf("Display() = %q, want \"The Curia\"", got)
	}
	if got := (Body{Name: "Cardano Curia"}).Display(); got != "Cardano Curia" {
		t.Errorf("Display() with no short name = %q, want the full name", got)
	}
}

// A member's handle becomes a link without the roster having to spell one out.
func TestMemberLink(t *testing.T) {
	if got := (Member{Handle: "@CullahMusic"}).Link(); got != "https://x.com/CullahMusic" {
		t.Errorf("Link() = %q", got)
	}
	// An explicit URL wins, so a member on something other than X is not forced onto it.
	m := Member{Handle: "@someone", HandleURL: "https://mastodon.social/@someone"}
	if got := m.Link(); got != "https://mastodon.social/@someone" {
		t.Errorf("an explicit handleUrl was ignored: %q", got)
	}
	if got := (Member{}).Link(); got != "" {
		t.Errorf("a member with no handle got a link: %q", got)
	}
}

// Both shipped examples must actually load — an example roster that does not
// parse is worse than none, because it is the thing an operator copies.
func TestExampleRostersLoad(t *testing.T) {
	for _, name := range []string{"body/curia.json", "body/solo.example.json"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", name)
			if _, err := os.Stat(path); err != nil {
				t.Skipf("%s not present", name)
			}
			b, err := LoadBody(path)
			if err != nil {
				t.Fatalf("LoadBody(%s): %v", name, err)
			}
			if b.Name == "" {
				t.Error("the example body has no name")
			}
			if len(b.Members) == 0 {
				t.Error("the example body has no members")
			}
			for _, m := range b.Members {
				if m.Name == "" {
					t.Error("a member in the example has no name")
				}
			}
		})
	}

	// The solo example must actually be solo, or it is not demonstrating the case.
	b, err := LoadBody(filepath.Join("..", "..", "body/solo.example.json"))
	if err == nil && !b.Solo() {
		t.Error("the solo example roster is not a solo body")
	}
}
