package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// signChallenge builds a CIP-8 COSE_Sign1 + COSE_Key the way a Cardano wallet's
// signData would, so we can exercise verification without a real wallet.
func signChallenge(t *testing.T, challengeHex string) (sigHex, keyHex string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := hex.DecodeString(challengeHex)
	if err != nil {
		t.Fatal(err)
	}
	protected, err := cbor.Marshal(map[any]any{int(1): int(-8), "address": []byte{0x01, 0x02, 0x03}})
	if err != nil {
		t.Fatal(err)
	}
	sigStruct, err := cbor.Marshal([]any{"Signature1", protected, []byte{}, payload})
	if err != nil {
		t.Fatal(err)
	}
	sig := ed25519.Sign(priv, sigStruct)

	cose, err := cbor.Marshal([]any{protected, map[any]any{}, payload, sig})
	if err != nil {
		t.Fatal(err)
	}
	key, err := cbor.Marshal(map[int]any{1: 1, 3: -8, -1: 6, -2: []byte(pub)})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(cose), hex.EncodeToString(key)
}

func TestVerifyCOSESign1(t *testing.T) {
	ch := "0011223344556677889900aabbccddeeff00112233445566"
	sigHex, keyHex := signChallenge(t, ch)

	payload, ok, err := verifyCOSESign1(sigHex, keyHex)
	if err != nil || !ok {
		t.Fatalf("verify failed: ok=%v err=%v", ok, err)
	}
	if hex.EncodeToString(payload) != ch {
		t.Errorf("payload = %s, want %s", hex.EncodeToString(payload), ch)
	}
}

func TestVerifyCOSESign1RejectsTamper(t *testing.T) {
	ch := "0011223344556677889900aabbccddeeff00112233445566"
	sigHex, keyHex := signChallenge(t, ch)

	b, _ := hex.DecodeString(sigHex)
	b[len(b)-1] ^= 0xff // corrupt the signature
	if _, ok, _ := verifyCOSESign1(hex.EncodeToString(b), keyHex); ok {
		t.Error("a tampered signature verified")
	}
}

func TestChallengeSingleUse(t *testing.T) {
	h, err := issueChallenge()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := hex.DecodeString(h)
	if !challengeMatches(raw) {
		t.Fatal("a freshly issued challenge did not match")
	}
	if challengeMatches(raw) {
		t.Error("a challenge should be single-use (consumed on first match)")
	}
}
