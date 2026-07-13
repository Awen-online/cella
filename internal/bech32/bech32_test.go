package bech32

import (
	"encoding/hex"
	"strings"
	"testing"
)

// Cardano addresses are longer than BIP-173's 90-character limit, so a decoder
// that enforces it rejects every real mainnet address. This is the case that
// matters most.
func TestDecodeLongCardanoAddress(t *testing.T) {
	const addr = "addr1q8ejkg9t0tkqxkms3nqe2e90rgdn680mg9vfq5ygd06j94d0xdpt8x0tr7mpserrhsssmh0d8ug7494ndwr53rcs3veq6dulc9"
	if len(addr) <= 90 {
		t.Fatalf("test address is %d chars; it must exceed BIP-173's 90-char limit to be meaningful", len(addr))
	}

	hrp, data, err := Decode(addr)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if hrp != "addr" {
		t.Errorf("hrp = %q, want \"addr\"", hrp)
	}
	// A base address is 1 header byte + two 28-byte credentials.
	if len(data) != 57 {
		t.Errorf("decoded %d bytes, want 57", len(data))
	}
	if got := hex.EncodeToString(data[1:29]); got != "f32b20ab7aec035b708cc19564af1a1b3d1dfb41589050886bf522d5" {
		t.Errorf("payment credential = %s", got)
	}
}

func TestRoundTrip(t *testing.T) {
	raw, err := hex.DecodeString("e1af3342b399eb1fb6186463bc210ddded3f11ea96b36b87488f108b32")
	if err != nil {
		t.Fatal(err)
	}

	addr, err := Encode("stake", raw)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	// Cross-checked independently: this is the reward address for the stake
	// credential of the base address above.
	const want = "stake1uxhnxs4nn843ldscv33mcggdmhkn7y02j6ekhp6g3uggkvsu2jmuk"
	if addr != want {
		t.Errorf("Encode = %s, want %s", addr, want)
	}

	hrp, back, err := Decode(addr)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if hrp != "stake" || hex.EncodeToString(back) != hex.EncodeToString(raw) {
		t.Errorf("round trip lost data: hrp=%q data=%x", hrp, back)
	}
}

// The checksum is the only thing standing between a typo and a delegate being
// mapped to the wrong credential, so every corruption must be caught.
func TestDecodeRejectsCorruption(t *testing.T) {
	const good = "stake1uxhnxs4nn843ldscv33mcggdmhkn7y02j6ekhp6g3uggkvsu2jmuk"
	if _, _, err := Decode(good); err != nil {
		t.Fatalf("the control address must decode: %v", err)
	}

	cases := map[string]string{
		"flipped character": strings.Replace(good, "x", "z", 1),
		"truncated":         good[:len(good)-1],
		"extra character":   good + "q",
		"no separator":      strings.ReplaceAll(good, "1", "q"),
		"mixed case":        "Stake1uxhnxs4nn843ldscv33mcggdmhkn7y02j6ekhp6g3uggkvsu2jmuk",
		"out-of-charset b":  strings.Replace(good, "u", "b", 1),
		"empty":             "",
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Decode(bad); err == nil {
				t.Errorf("Decode(%q) succeeded; want an error", bad)
			}
		})
	}
}
