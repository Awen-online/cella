package cardano

import (
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/blake2b"
)

// A real credential-manager hot NFT datum, naming three voting users.
//
// This is not a hand-rolled fixture: its blake2b-256 is
// bcaef52050aed8a9720c1854860aee8625f3655fdeb8697a5650e257d7c56fc9, which is
// the inline datum hash IntersectMBO publishes for this UTxO in the
// credential-manager orchestrator docs. TestFixtureIsGenuine checks that,
// so if the bytes below are ever wrong the rest of this file stops meaning
// anything and says so.
const hotDatumCBOR = "9f" +
	"d8799f581cc6731b9c6de6bf11d91f08099953cb393505806ff522e5cc3a7574ab" +
	"5820e50384c655f9a33cabf64e41df7282e765a242aef182130f1db01bce8859e0aaff" +
	"d8799f581cc6d6ffd8e93b1b8352c297d528c958b982098dc8a08025bbb8d864cf" +
	"5820e3340359f5d25c051e4dd160e4cb4d75074c537905f07eb9a2e24db881246ee0ff" +
	"d8799f581c2faaa04cee79d9abfa3149c814617e860567a8609bbfbd044566a5cd" +
	"5820ae8eef56d67350b247ab77be48dad121ae18d473386f59b3fda9fccbd665422aff" +
	"ff"

const publishedDatumHash = "bcaef52050aed8a9720c1854860aee8625f3655fdeb8697a5650e257d7c56fc9"

// The fixture must be the real thing, not something that merely round-trips
// through this package's own assumptions.
func TestFixtureIsGenuine(t *testing.T) {
	raw, err := hex.DecodeString(hotDatumCBOR)
	if err != nil {
		t.Fatalf("fixture is not hex: %v", err)
	}
	sum := blake2b.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != publishedDatumHash {
		t.Fatalf("fixture hashes to %s, but IntersectMBO publishes %s for this datum",
			got, publishedDatumHash)
	}
}

func TestDecodeHotDatum(t *testing.T) {
	group, err := DecodeHotDatum(hotDatumCBOR)
	if err != nil {
		t.Fatalf("DecodeHotDatum: %v", err)
	}
	if len(group) != 3 {
		t.Fatalf("decoded %d voting users, want 3", len(group))
	}

	want := []VotingIdentity{
		{"c6731b9c6de6bf11d91f08099953cb393505806ff522e5cc3a7574ab",
			"e50384c655f9a33cabf64e41df7282e765a242aef182130f1db01bce8859e0aa"},
		{"c6d6ffd8e93b1b8352c297d528c958b982098dc8a08025bbb8d864cf",
			"e3340359f5d25c051e4dd160e4cb4d75074c537905f07eb9a2e24db881246ee0"},
		{"2faaa04cee79d9abfa3149c814617e860567a8609bbfbd044566a5cd",
			"ae8eef56d67350b247ab77be48dad121ae18d473386f59b3fda9fccbd665422a"},
	}
	for i, w := range want {
		if group[i] != w {
			t.Errorf("voting user %d = %+v, want %+v", i, group[i], w)
		}
	}

	if !group.Has(want[1].KeyHash) {
		t.Error("Has() did not find a key hash that is in the group")
	}
	if group.Has("00000000000000000000000000000000000000000000000000000000") {
		t.Error("Has() found a key hash that is not in the group")
	}
}

// The validator requires ceil(n/2) signatures — half, rounded up. This is NOT
// "more than half": a group of four needs two, not three. Getting it wrong in
// the intuitive direction would have Cella tell a committee it was a signature
// short of a quorum it had already reached.
func TestQuorumIsHalfRoundedUp(t *testing.T) {
	cases := map[int]int{
		1: 1,
		2: 1, // not 2
		3: 2,
		4: 2, // not 3 — the case that catches a floor(n/2)+1 implementation
		5: 3,
		6: 3, // not 4
		7: 4,
		8: 4,
		9: 5,
	}
	for n, want := range cases {
		g := make(VotingGroup, 0, n)
		for i := 0; i < n; i++ {
			g = append(g, VotingIdentity{KeyHash: hex.EncodeToString([]byte{byte(i)})})
		}
		if got := g.Quorum(); got != want {
			t.Errorf("a group of %d needs %d signatures, want %d", n, got, want)
		}
	}

	var empty VotingGroup
	if got := empty.Quorum(); got != 0 {
		t.Errorf("empty group quorum = %d, want 0", got)
	}
}

// The validator dedupes the group before counting, so a key listed twice must
// not inflate the quorum.
func TestQuorumCountsDistinctKeys(t *testing.T) {
	dup := VotingGroup{
		{KeyHash: "aa", CertHash: "01"},
		{KeyHash: "aa", CertHash: "02"}, // same key, different certificate
		{KeyHash: "bb", CertHash: "03"},
	}
	if got := len(dup.Distinct()); got != 2 {
		t.Errorf("Distinct() = %d keys, want 2", got)
	}
	// Two distinct keys → quorum of 1, not 2 (which 3 raw entries would give).
	if got := dup.Quorum(); got != 1 {
		t.Errorf("Quorum() = %d, want 1 (ceil(2/2)) — duplicates must not inflate it", got)
	}
}

func TestDecodeHotDatumRejectsRubbish(t *testing.T) {
	cases := map[string]string{
		"not hex":            "zzzz",
		"empty":              "",
		"not a list":         "d8799f4101ff",            // a bare constructor, not a list of them
		"empty list":         "80",                      // decodes, but names nobody
		"wrong tag":          "9fd87a9f581c" + "00",     // constructor 1, truncated
		"short key hash":     "9fd8799f41015820" + "00", // 1-byte key hash
		"plain array member": "9f9f41014102ffff",        // untagged pair, not a constructor
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if g, err := DecodeHotDatum(in); err == nil {
				t.Errorf("DecodeHotDatum(%q) succeeded with %d users; want an error", in, len(g))
			}
		})
	}
}
