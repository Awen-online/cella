package server

import (
	"crypto/ed25519"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// signVote produces the wallet signature a delegate would give for a position.
func signVote(t *testing.T, s *Server, priv ed25519.PrivateKey, proposalID, vote, rationale string) (sigHex, keyHex string) {
	t.Helper()
	msg := voteMessage(s.body.Name, proposalID, vote, rationale)
	return signChallenge(t, priv, hex.EncodeToString([]byte(msg)))
}

// castSigned posts a ballot carrying a wallet signature.
func castSigned(t *testing.T, s *Server, slug, identity, vote, rationale, sig, key string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{
		"slug":      {slug},
		"vote":      {vote},
		"rationale": {rationale},
		"csrf":      {s.csrfToken(identity)},
		"signature": {sig},
		"key":       {key},
	}
	r := httptest.NewRequest(http.MethodPost, "/vote", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(session(s, identity))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)
	return rec
}

func TestSignedVoteIsRecorded(t *testing.T) {
	pub, priv := newKeypair(t)
	s := walletServer(t, pub) // roster: Junia Marcia owns this wallet
	slug, pid := slugOf(s, t)

	sig, key := signVote(t, s, priv, pid, "No", "Article IV is not satisfied.")
	rec := castSigned(t, s, slug, "Junia Marcia", "No", "Article IV is not satisfied.", sig, key)
	if rec.Code != http.StatusFound {
		t.Fatalf("signed vote = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}

	votes, err := s.db.MemberVotesFor(pid)
	if err != nil {
		t.Fatalf("read votes: %v", err)
	}
	got, ok := votes["Junia Marcia"]
	if !ok {
		t.Fatal("the vote was not recorded")
	}
	if got.Vote != "No" {
		t.Errorf("vote = %q, want No", got.Vote)
	}
	if !got.Signed() {
		t.Error("the recorded vote carries no signature")
	}
}

// The signature covers the position. Recording a *different* position while
// presenting a signature over an earlier one must fail — otherwise a delegate
// could be shown one thing and have another recorded in their name.
func TestSignatureMustCoverTheRecordedPosition(t *testing.T) {
	pub, priv := newKeypair(t)
	s := walletServer(t, pub)
	slug, pid := slugOf(s, t)

	// Signed "Yes", but submitted as "No".
	sig, key := signVote(t, s, priv, pid, "Yes", "Sound.")

	cases := []struct {
		name            string
		vote, rationale string
	}{
		{"vote changed", "No", "Sound."},
		{"rationale changed", "Yes", "Actually, unsound."},
		{"both changed", "Abstain", "Recusing."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := castSigned(t, s, slug, "Junia Marcia", tc.vote, tc.rationale, sig, key)
			if rec.Code != http.StatusForbidden {
				t.Errorf("tampered position = %d, want 403", rec.Code)
			}
			if votes, _ := s.db.MemberVotesFor(pid); len(votes) != 0 {
				t.Errorf("a tampered position was recorded anyway: %v", votes)
			}
		})
	}
}

// A signature over one action must not be replayable onto another.
func TestSignatureIsBoundToTheAction(t *testing.T) {
	pub, priv := newKeypair(t)
	s := walletServer(t, pub)
	slug, pid := slugOf(s, t)

	// A signature over a different proposal id.
	sig, key := signVote(t, s, priv, "gov_action_somewhere_else#3", "Yes", "Sound.")

	rec := castSigned(t, s, slug, "Junia Marcia", "Yes", "Sound.", sig, key)
	if rec.Code != http.StatusForbidden {
		t.Errorf("replayed signature = %d, want 403", rec.Code)
	}
	if votes, _ := s.db.MemberVotesFor(pid); len(votes) != 0 {
		t.Error("a signature from another action was accepted")
	}
}

// One delegate must not be able to sign a position into another's name, even
// holding a valid session for the second.
func TestCannotSignIntoAnotherDelegatesName(t *testing.T) {
	pub, _ := newKeypair(t)
	s := walletServer(t, pub) // Junia Marcia's wallet
	slug, pid := slugOf(s, t)

	// A stranger's key signs a position, submitted under Junia Marcia's session.
	_, strangerPriv := newKeypair(t)
	sig, key := signVote(t, s, strangerPriv, pid, "Yes", "Sound.")

	rec := castSigned(t, s, slug, "Junia Marcia", "Yes", "Sound.", sig, key)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a foreign signature = %d, want 403", rec.Code)
	}
	if votes, _ := s.db.MemberVotesFor(pid); len(votes) != 0 {
		t.Error("a position signed by someone else's key was recorded")
	}
}

// Re-recording without signing must not leave the old signature attached — a
// signature over a superseded position says nothing about the new one.
func TestResubmittingUnsignedClearsTheSignature(t *testing.T) {
	pub, priv := newKeypair(t)
	s := walletServer(t, pub)
	slug, pid := slugOf(s, t)

	sig, key := signVote(t, s, priv, pid, "Yes", "Sound.")
	if rec := castSigned(t, s, slug, "Junia Marcia", "Yes", "Sound.", sig, key); rec.Code != http.StatusFound {
		t.Fatalf("initial signed vote = %d, want 302", rec.Code)
	}
	if v, _ := s.db.MemberVotesFor(pid); !v["Junia Marcia"].Signed() {
		t.Fatal("the signed vote was not stored as signed")
	}

	// Now change the position with no signature at all.
	if rec := castSigned(t, s, slug, "Junia Marcia", "No", "Changed my mind.", "", ""); rec.Code != http.StatusFound {
		t.Fatalf("unsigned update = %d, want 302", rec.Code)
	}
	got, _ := s.db.MemberVotesFor(pid)
	if got["Junia Marcia"].Vote != "No" {
		t.Errorf("vote = %q, want No", got["Junia Marcia"].Vote)
	}
	if got["Junia Marcia"].Signed() {
		t.Error("the old signature survived onto a new, unsigned position")
	}
}

// The message a delegate signs must name the action, the vote and the rationale
// — everything that gives the position meaning must be inside the signature.
func TestVoteMessageBindsEverythingThatMatters(t *testing.T) {
	msg := voteMessage("Cardano Curia", "gov_action1abc#0", "No", "Article IV is not satisfied.")
	for _, want := range []string{
		"Cardano Curia",
		"gov_action1abc#0",
		"No",
		"Article IV is not satisfied.",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the signed message does not bind %q:\n%s", want, msg)
		}
	}

	// It must be stable — the server rebuilds it to verify, so any drift breaks
	// every signature.
	if msg != voteMessage("Cardano Curia", "gov_action1abc#0", "No", "Article IV is not satisfied.") {
		t.Error("voteMessage is not deterministic")
	}

	// Different positions must produce different messages, or a signature over
	// one would verify against another.
	distinct := map[string]bool{
		voteMessage("B", "a1", "Yes", "r"):     true,
		voteMessage("B", "a1", "No", "r"):      true,
		voteMessage("B", "a1", "Abstain", "r"): true,
		voteMessage("B", "a2", "Yes", "r"):     true,
		voteMessage("B", "a1", "Yes", "other"): true,
		voteMessage("C", "a1", "Yes", "r"):     true,
	}
	if len(distinct) != 6 {
		t.Errorf("distinct positions collided into %d messages, want 6", len(distinct))
	}
}

// The prepare endpoint hands back exactly the bytes the verifier will rebuild.
func TestVotePrepareMatchesVerification(t *testing.T) {
	pub, _ := newKeypair(t)
	s := walletServer(t, pub)
	slug, pid := slugOf(s, t)

	form := url.Values{
		"slug":      {slug},
		"vote":      {"Yes"},
		"rationale": {"Proportionate."},
		"csrf":      {s.csrfToken("Junia Marcia")},
	}
	r := httptest.NewRequest(http.MethodPost, "/vote/prepare", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(session(s, "Junia Marcia"))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("prepare = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	want := voteMessage(s.body.Name, pid, "Yes", "Proportionate.")
	if !strings.Contains(body, hex.EncodeToString([]byte(want))) {
		t.Error("the prepared message is not the one the verifier will rebuild")
	}
}
