package config

import "testing"

// CELLA_DEMO disables authentication. It must be opted into deliberately and
// never tripped by a typo or a stray value, so anything unrecognised is false.
func TestTruthy(t *testing.T) {
	on := []string{"1", "true", "TRUE", "True", "yes", "YES", "on", " 1 ", "\ttrue\n"}
	for _, v := range on {
		if !truthy(v) {
			t.Errorf("truthy(%q) = false, want true", v)
		}
	}

	// Everything else is off. "0" and "false" obviously — but so is anything
	// misspelled, because a deployment that meant to leave demo mode off must
	// not have it switched on by a value nobody intended.
	off := []string{"", "0", "false", "no", "off", "TRUE1", "yes please", "y", "t", "enabled", "demo", "-1", "null"}
	for _, v := range off {
		if truthy(v) {
			t.Errorf("truthy(%q) = true, want false — an unrecognised value must not disable authentication", v)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	// Every value has a default, so Cella runs with zero configuration.
	t.Setenv("CELLA_DB", "")
	t.Setenv("CELLA_ADDR", "")
	t.Setenv("KOIOS_URL", "")
	t.Setenv("CELLA_DEMO", "")
	t.Setenv("CELLA_SECRET", "")
	t.Setenv("CELLA_BODY", "")
	t.Setenv("CELLA_ROSTER", "")
	t.Setenv("CELLA_HOT_NFT_ADDR", "")

	c := Load()
	if c.DBPath != "cella.db" {
		t.Errorf("DBPath = %q, want cella.db", c.DBPath)
	}
	if c.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", c.Addr)
	}
	if c.KoiosURL != "https://api.koios.rest/api/v1" {
		t.Errorf("KoiosURL = %q, want the public Koios endpoint", c.KoiosURL)
	}

	// The three that weaken or unlock things must default to off/empty.
	if c.Demo {
		t.Error("Demo defaults to true; authentication must be on unless asked otherwise")
	}
	if c.Secret != "" || c.BodyPath != "" || c.HotNFTAddr != "" {
		t.Errorf("secrets/paths should default empty, got %+v", c)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("CELLA_DB", "/tmp/x.db")
	t.Setenv("CELLA_ADDR", "127.0.0.1:9000")
	t.Setenv("CELLA_DEMO", "yes")
	t.Setenv("CELLA_SECRET", "s3cret")
	t.Setenv("CELLA_BODY", "body/curia.json")
	t.Setenv("CELLA_HOT_NFT_ADDR", "addr1w_hot")

	c := Load()
	if c.DBPath != "/tmp/x.db" || c.Addr != "127.0.0.1:9000" {
		t.Errorf("db/addr not read from the environment: %+v", c)
	}
	if !c.Demo {
		t.Error("CELLA_DEMO=yes did not enable demo mode")
	}
	if c.Secret != "s3cret" || c.BodyPath != "body/curia.json" || c.HotNFTAddr != "addr1w_hot" {
		t.Errorf("config not read from the environment: %+v", c)
	}
}
