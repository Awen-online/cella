package cardano

import (
	"encoding/hex"
	"strings"
	"testing"
)

// A real mainnet base address (Cullah's, from the delegate roster). Base
// addresses are 103 characters — longer than BIP-173's 90-character limit,
// which is exactly why the decoder must not enforce it.
const cullah = "addr1q8ejkg9t0tkqxkms3nqe2e90rgdn680mg9vfq5ygd06j94d0xdpt8x0tr7mpserrhsssmh0d8ug7494ndwr53rcs3veq6dulc9"

func TestCredentialsFromBaseAddress(t *testing.T) {
	got, err := Credentials(cullah)
	if err != nil {
		t.Fatalf("Credentials: %v", err)
	}
	// A base address carries both a payment and a stake credential.
	if len(got) != 2 {
		t.Fatalf("got %d credentials, want 2 (payment + stake): %v", len(got), got)
	}
	for _, c := range got {
		b, err := hex.DecodeString(c)
		if err != nil {
			t.Fatalf("credential %q is not hex: %v", c, err)
		}
		if len(b) != KeyHashLen {
			t.Errorf("credential %q is %d bytes, want %d", c, len(b), KeyHashLen)
		}
	}
	if got[0] == got[1] {
		t.Error("payment and stake credentials are identical; the address was misparsed")
	}
}

// The stake address derived from a base address must yield that base address's
// stake credential — this is the cross-check that proves the byte offsets are
// right rather than merely self-consistent.
func TestStakeAddressMatchesBaseAddressStakeCredential(t *testing.T) {
	base, err := Credentials(cullah)
	if err != nil {
		t.Fatalf("Credentials(base): %v", err)
	}

	// The reward address for the same account, built independently: header byte
	// 0xe1 (reward, mainnet) followed by the base address's stake credential,
	// re-encoded as bech32.
	const stakeAddr = "stake1uxhnxs4nn843ldscv33mcggdmhkn7y02j6ekhp6g3uggkvsu2jmuk"
	got, err := Credentials(stakeAddr)
	if err != nil {
		t.Fatalf("Credentials(stake): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("a reward address should carry exactly 1 credential, got %d: %v", len(got), got)
	}
	if got[0] != base[1] {
		t.Errorf("stake address credential = %s, but the base address's stake credential is %s",
			got[0], base[1])
	}
}

// A credential is blake2b-224 of the public key. Pin the digest so a change of
// hash length or algorithm — which would silently stop every delegate matching
// their own address — is caught here rather than at sign-in.
func TestKeyHashIsBlake2b224(t *testing.T) {
	h := KeyHash(make([]byte, 32)) // all-zero Ed25519 public key
	if len(h) != KeyHashLen {
		t.Fatalf("KeyHash length = %d, want %d", len(h), KeyHashLen)
	}
	const want = "f9dca21a6c826ec8acb4cf395cbc24351937bfe6560b2683ab8b415f"
	if got := hex.EncodeToString(h); got != want {
		t.Errorf("blake2b-224(32 zero bytes) = %s, want %s", got, want)
	}
}

func TestCredentialsRejectsGarbage(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"no separator":     "addrqqqqqq",
		"bad checksum":     strings.TrimSuffix(cullah, "9") + "x",
		"not bech32":       "0x1234567890abcdef",
		"too short":        "addr1qqqqqqqqqq",
		"invalid char (b)": strings.Replace(cullah, "q", "b", 1),
	}
	for name, addr := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Credentials(addr); err == nil {
				t.Errorf("Credentials(%q) succeeded; want an error", addr)
			}
		})
	}
}

// The demo roster's placeholder addresses are not real bech32. They must fail
// to decode rather than silently producing a credential that could match
// something.
func TestPlaceholderAddressesDoNotDecode(t *testing.T) {
	const fake = "stake1uy9v3k7m2q0f8xw4r6p2n5c8t3l7d1s4h9j0a2b6e5g8c9q7wq2demo01"
	if _, err := Credentials(fake); err == nil {
		t.Error("a placeholder demo address decoded as a real credential")
	}
}
