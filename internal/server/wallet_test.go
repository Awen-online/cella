package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Awen-online/cella/internal/bech32"
	"github.com/Awen-online/cella/internal/cardano"
	"github.com/fxamacker/cbor/v2"
)

// rewardAddress builds the mainnet reward address for an Ed25519 public key:
// header byte 0xe1 followed by the key's blake2b-224 credential. This is how a
// delegate's roster address relates to the key their wallet signs with.
func rewardAddress(t *testing.T, pub ed25519.PublicKey) string {
	t.Helper()
	addr, err := bech32.Encode("stake", append([]byte{0xe1}, cardano.KeyHash(pub)...))
	if err != nil {
		t.Fatalf("encode reward address: %v", err)
	}
	return addr
}

// signChallenge builds a CIP-8 COSE_Sign1 + COSE_Key the way a Cardano wallet's
// signData would, so we can exercise verification without a real wallet.
func signChallenge(t *testing.T, priv ed25519.PrivateKey, challengeHex string) (sigHex, keyHex string) {
	t.Helper()
	pub := priv.Public().(ed25519.PublicKey)

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

func newKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestVerifyCOSESign1(t *testing.T) {
	pub, priv := newKeypair(t)
	const ch = "0011223344556677889900aabbccddeeff00112233445566"
	sigHex, keyHex := signChallenge(t, priv, ch)

	payload, gotPub, ok, err := verifyCOSESign1(sigHex, keyHex)
	if err != nil || !ok {
		t.Fatalf("verify failed: ok=%v err=%v", ok, err)
	}
	if hex.EncodeToString(payload) != ch {
		t.Errorf("payload = %s, want %s", hex.EncodeToString(payload), ch)
	}
	// The public key must come back out, because it is what identifies the signer.
	if !ed25519.PublicKey(gotPub).Equal(pub) {
		t.Error("verifyCOSESign1 did not return the signing public key")
	}
}

func TestVerifyCOSESign1RejectsTamper(t *testing.T) {
	_, priv := newKeypair(t)
	sigHex, keyHex := signChallenge(t, priv, "0011223344556677889900aabbccddeeff00112233445566")

	b, _ := hex.DecodeString(sigHex)
	b[len(b)-1] ^= 0xff // corrupt the signature
	if _, _, ok, _ := verifyCOSESign1(hex.EncodeToString(b), keyHex); ok {
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

// walletServer returns a server whose roster contains one delegate, identified
// by the given wallet key.
func walletServer(t *testing.T, pub ed25519.PublicKey) *Server {
	t.Helper()
	s, _ := seedServer(t)
	s.body = Body{
		Name: "Test Body",
		Members: []Member{
			{Name: "Junia Marcia", Role: "Delegate", Address: rewardAddress(t, pub)},
			{Name: "Titus Varo", Role: "Delegate"}, // no wallet registered
		},
	}
	return s
}

// verifyWallet drives POST /auth/verify for a wallet signing a live challenge.
func verifyWallet(t *testing.T, s *Server, priv ed25519.PrivateKey, claimedAddr string) *httptest.ResponseRecorder {
	t.Helper()
	ch, err := issueChallenge()
	if err != nil {
		t.Fatal(err)
	}
	sigHex, keyHex := signChallenge(t, priv, ch)

	body, _ := json.Marshal(verifyReq{Address: claimedAddr, Signature: sigHex, Key: keyHex})
	r := httptest.NewRequest(http.MethodPost, "/auth/verify", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)
	return rec
}

// A wallet on the roster signs in as the delegate that wallet belongs to — not,
// as it did before, as whichever delegate happened to be hardcoded.
func TestWalletSignsInAsItsOwnDelegate(t *testing.T) {
	pub, priv := newKeypair(t)
	s := walletServer(t, pub)

	rec := verifyWallet(t, s, priv, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie issued")
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(cookies[0])
	got, ok := s.member(r)
	if !ok || got != "Junia Marcia" {
		t.Errorf("signed in as %q, want \"Junia Marcia\" (the delegate who owns the wallet)", got)
	}
}

// A wallet nobody on the roster owns must not get in at all. Before, any wallet
// that could produce a valid signature was admitted as a delegate.
func TestUnknownWalletIsRefused(t *testing.T) {
	pub, _ := newKeypair(t)
	s := walletServer(t, pub)

	_, strangerPriv := newKeypair(t) // a different key, not on the roster
	rec := verifyWallet(t, s, strangerPriv, "")

	if rec.Code != http.StatusForbidden {
		t.Errorf("an off-roster wallet = %d, want 403", rec.Code)
	}
	if len(rec.Result().Cookies()) > 0 {
		t.Error("an off-roster wallet was issued a session")
	}
}

// The identity comes from the key inside the signature, never from the address
// the client sends alongside it — otherwise a stranger could sign with their
// own key while claiming a delegate's address.
func TestClaimedAddressIsIgnored(t *testing.T) {
	pub, _ := newKeypair(t)
	s := walletServer(t, pub)
	delegateAddr := s.body.Members[0].Address

	_, strangerPriv := newKeypair(t)
	rec := verifyWallet(t, s, strangerPriv, delegateAddr) // claims to be Junia Marcia

	if rec.Code != http.StatusForbidden {
		t.Errorf("a stranger claiming a delegate's address = %d, want 403", rec.Code)
	}
	if len(rec.Result().Cookies()) > 0 {
		t.Error("impersonation by claimed address succeeded")
	}
}

// A signature that does not answer a live challenge must be rejected, so a
// captured one cannot be replayed.
func TestReplayedSignatureIsRefused(t *testing.T) {
	pub, priv := newKeypair(t)
	s := walletServer(t, pub)

	ch, err := issueChallenge()
	if err != nil {
		t.Fatal(err)
	}
	sigHex, keyHex := signChallenge(t, priv, ch)
	body, _ := json.Marshal(verifyReq{Signature: sigHex, Key: keyHex})

	post := func() int {
		r := httptest.NewRequest(http.MethodPost, "/auth/verify", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.mux.ServeHTTP(rec, r)
		return rec.Code
	}

	if code := post(); code != http.StatusOK {
		t.Fatalf("first use of a challenge = %d, want 200", code)
	}
	if code := post(); code == http.StatusOK {
		t.Error("the same signature was accepted twice; challenges must be single-use")
	}
}

func TestByCredential(t *testing.T) {
	pub, _ := newKeypair(t)
	body := Body{Members: []Member{
		{Name: "Junia Marcia", Address: rewardAddress(t, pub)},
		{Name: "Titus Varo"},
	}}

	m, ok := body.ByCredential(hex.EncodeToString(cardano.KeyHash(pub)))
	if !ok || m.Name != "Junia Marcia" {
		t.Errorf("ByCredential = %q, %v; want \"Junia Marcia\", true", m.Name, ok)
	}

	otherPub, _ := newKeypair(t)
	if m, ok := body.ByCredential(hex.EncodeToString(cardano.KeyHash(otherPub))); ok {
		t.Errorf("an unknown credential matched %q", m.Name)
	}
	// A delegate with no registered address matches nothing, least of all "".
	if m, ok := body.ByCredential(""); ok {
		t.Errorf("the empty credential matched %q", m.Name)
	}
}
