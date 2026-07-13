package network

import "testing"

func TestParse(t *testing.T) {
	for in, want := range map[string]Network{
		"":          Mainnet, // the default
		"mainnet":   Mainnet,
		"MAINNET":   Mainnet,
		" preprod ": Preprod,
		"preview":   Preview,
	} {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

// A misspelled network must be an error, not a silent mainnet. An operator who
// meant to practise and quietly ended up on mainnet would be voting for real.
func TestParseRefusesTheUnknown(t *testing.T) {
	for _, bad := range []string{"mainet", "testnet", "prod", "preprd", "guild"} {
		if got, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) = %q with no error; want a refusal", bad, got)
		}
	}
}

// SanchoNet gets a specific answer, because "unknown network" would send an
// operator looking for a typo they did not make. Koios does not serve it.
func TestSanchoNetSaysWhy(t *testing.T) {
	_, err := Parse("sanchonet")
	if err == nil {
		t.Fatal("SanchoNet was accepted; Koios does not serve it")
	}
	for _, want := range []string{"Koios does not serve it", "preprod"} {
		if !contains(err.Error(), want) {
			t.Errorf("the SanchoNet error does not mention %q: %v", want, err)
		}
	}
}

func TestKoiosAndExplorerFollowTheNetwork(t *testing.T) {
	cases := map[Network]struct{ koios, explorer string }{
		Mainnet: {"https://api.koios.rest/api/v1", "https://adastat.net/governances/abc00"},
		Preprod: {"https://preprod.koios.rest/api/v1", "https://preprod.adastat.net/governances/abc00"},
		Preview: {"https://preview.koios.rest/api/v1", "https://preview.adastat.net/governances/abc00"},
	}
	for n, want := range cases {
		if got := n.KoiosURL(); got != want.koios {
			t.Errorf("%s KoiosURL = %q, want %q", n, got, want.koios)
		}
		// An explorer link that stayed on mainnet would send a delegate to check
		// an action that does not exist on the chain they are actually using.
		if got := n.ExplorerAction("abc00"); got != want.explorer {
			t.Errorf("%s ExplorerAction = %q, want %q", n, got, want.explorer)
		}
	}

	if !Preprod.IsTestnet() || !Preview.IsTestnet() || Mainnet.IsTestnet() {
		t.Error("IsTestnet is wrong somewhere")
	}
	if Preprod.AddressPrefix() != "addr_test" || Mainnet.AddressPrefix() != "addr" {
		t.Error("address prefixes do not follow the network")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
