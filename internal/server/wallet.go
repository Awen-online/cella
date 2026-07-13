package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Awen-online/cella/internal/cardano"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

// CIP-30 "sign to log in": the browser connects a Cardano wallet, signs a
// server-issued challenge (CIP-8 / COSE_Sign1), and the server verifies the
// Ed25519 signature to prove the visitor controls the wallet key. No funds move
// and no key ever leaves the wallet.

// challengeTTL bounds how long an issued challenge is valid.
const challengeTTL = 5 * time.Minute

var (
	challMu sync.Mutex
	challs  = map[string]int64{} // challenge hex -> expiry unix
)

func issueChallenge() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	h := hex.EncodeToString(b)
	challMu.Lock()
	challs[h] = time.Now().Add(challengeTTL).Unix()
	// opportunistic sweep
	now := time.Now().Unix()
	for k, exp := range challs {
		if exp < now {
			delete(challs, k)
		}
	}
	challMu.Unlock()
	return h, nil
}

func consumeChallenge(h string) bool {
	challMu.Lock()
	defer challMu.Unlock()
	exp, ok := challs[h]
	if !ok || exp < time.Now().Unix() {
		delete(challs, h)
		return false
	}
	delete(challs, h)
	return true
}

// handleChallenge issues a fresh sign-in challenge.
func (s *Server) handleChallenge(w http.ResponseWriter, r *http.Request) {
	h, err := issueChallenge()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"challenge": h})
}

type verifyReq struct {
	Address   string `json:"address"`   // hex (from the wallet)
	Signature string `json:"signature"` // COSE_Sign1, hex
	Key       string `json:"key"`       // COSE_Key, hex
}

// handleVerify checks a signed challenge and, on success, starts a session.
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req verifyReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}

	payload, pub, ok, err := verifyCOSESign1(req.Signature, req.Key)
	if err != nil || !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "signature did not verify"})
		return
	}

	// The signed payload must be a live challenge — hashed or raw, depending on
	// the wallet. Consuming it here is what stops a captured signature being
	// replayed.
	if !challengeMatches(payload) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "challenge expired or unknown"})
		return
	}

	// Who signed? Not who they *say* they are — req.Address is attacker-
	// controlled and is deliberately ignored. The public key is inside the
	// signature we just verified, so hashing it yields a credential the signer
	// has actually proved they hold. That credential must belong to a delegate
	// on the roster.
	member, ok := s.body.ByCredential(hex.EncodeToString(cardano.KeyHash(pub)))
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "this wallet is not on the delegate roster",
		})
		return
	}

	s.setMember(w, member.Name)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1", "redirect": "/"})
}

// challengeMatches accepts either the raw challenge bytes or their blake2b-224
// hash (CIP-8 hashed payloads), consuming the matching challenge.
func challengeMatches(payload []byte) bool {
	// raw
	if consumeChallenge(hex.EncodeToString(payload)) {
		return true
	}
	// hashed: find a live challenge whose blake2b-224 equals the payload
	challMu.Lock()
	now := time.Now().Unix()
	var hit string
	for k, exp := range challs {
		if exp < now {
			continue
		}
		raw, err := hex.DecodeString(k)
		if err != nil {
			continue
		}
		sum := blake2b224(raw)
		if len(payload) == len(sum) && subtleEqual(payload, sum) {
			hit = k
			break
		}
	}
	if hit != "" {
		delete(challs, hit)
	}
	challMu.Unlock()
	return hit != ""
}

// verifyCOSESign1 verifies a CIP-8 COSE_Sign1 against the COSE_Key public key.
// On success it returns both the signed payload and the public key that signed
// it — the key is what identifies the signer, so callers must take it from here
// rather than from anything the client asserted separately.
func verifyCOSESign1(sigHex, keyHex string) (payload, pub []byte, ok bool, err error) {
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, nil, false, err
	}
	keyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, nil, false, err
	}

	// Strip a COSE_Sign1 tag (18 = 0xd2) if present.
	if len(sigBytes) > 0 && sigBytes[0] == 0xd2 {
		sigBytes = sigBytes[1:]
	}

	var sign1 struct {
		_           struct{} `cbor:",toarray"`
		Protected   []byte
		Unprotected cbor.RawMessage
		Payload     []byte
		Signature   []byte
	}
	if err := cbor.Unmarshal(sigBytes, &sign1); err != nil {
		return nil, nil, false, err
	}

	pub, err = coseKeyPub(keyBytes)
	if err != nil {
		return nil, nil, false, err
	}
	if len(pub) != ed25519.PublicKeySize {
		return nil, nil, false, nil
	}

	// Sig_structure = [ "Signature1", protected, external_aad(empty), payload ]
	sigStruct, err := cbor.Marshal([]interface{}{"Signature1", sign1.Protected, []byte{}, sign1.Payload})
	if err != nil {
		return nil, nil, false, err
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), sigStruct, sign1.Signature) {
		return nil, nil, false, nil
	}
	return sign1.Payload, pub, true, nil
}

// coseKeyPub extracts the Ed25519 public key (COSE_Key label -2) from a COSE_Key.
func coseKeyPub(b []byte) ([]byte, error) {
	m := map[int]cbor.RawMessage{}
	if err := cbor.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	raw, ok := m[-2]
	if !ok {
		return nil, errNoKey
	}
	var pub []byte
	if err := cbor.Unmarshal(raw, &pub); err != nil {
		return nil, err
	}
	return pub, nil
}

func blake2b224(b []byte) []byte {
	h, _ := blake2b.New(28, nil)
	h.Write(b)
	return h.Sum(nil)
}

func subtleEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func shortAddr(a string) string {
	if len(a) <= 14 {
		return a
	}
	return a[:8] + "…" + a[len(a)-6:]
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

type constErr string

func (e constErr) Error() string { return string(e) }

const errNoKey = constErr("no public key in COSE_Key")
