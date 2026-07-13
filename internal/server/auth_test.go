package server

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// session returns a signed cookie for identity, as the server would mint it.
func session(s *Server, identity string) *http.Cookie {
	rec := httptest.NewRecorder()
	s.setMember(rec, identity)
	return rec.Result().Cookies()[0]
}

func TestSessionRoundTrip(t *testing.T) {
	s, _ := seedServer(t)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(session(s, "Junia Marcia"))

	got, ok := s.member(r)
	if !ok || got != "Junia Marcia" {
		t.Fatalf("member() = %q, %v; want \"Junia Marcia\", true", got, ok)
	}
}

// A session cookie is signed precisely so that a visitor cannot hand-write one
// naming any delegate they like and then vote as them.
func TestForgedSessionIsRejected(t *testing.T) {
	s, _ := seedServer(t)

	forgeries := map[string]string{
		"unsigned plaintext":  "Junia Marcia",
		"unsigned base64":     base64.RawURLEncoding.EncodeToString([]byte("Junia Marcia")),
		"garbage signature":   base64.RawURLEncoding.EncodeToString([]byte("Junia Marcia")) + ".not-a-real-signature",
		"empty signature":     base64.RawURLEncoding.EncodeToString([]byte("Junia Marcia")) + ".",
		"signature from else": base64.RawURLEncoding.EncodeToString([]byte("Junia Marcia")) + "." + s.sign("Titus Varo"),
	}

	for name, value := range forgeries {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: value})
			if got, ok := s.member(r); ok {
				t.Errorf("forged cookie %q accepted as %q; want rejected", value, got)
			}
		})
	}
}

// A key change must invalidate existing sessions — otherwise the signature is
// decorative.
func TestSessionDoesNotVerifyUnderAnotherKey(t *testing.T) {
	s, _ := seedServer(t)
	other := New(s.db, "a-different-secret")

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(session(s, "Junia Marcia"))

	if _, ok := other.member(r); ok {
		t.Error("session signed with one key verified under another")
	}
}

func TestGateRedirectsWithoutSession(t *testing.T) {
	s, _ := seedServer(t)

	rec := httptest.NewRecorder()
	s.gate(s.mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/enter" {
		t.Errorf("gate = %d -> %q; want 302 -> /enter", rec.Code, rec.Header().Get("Location"))
	}
}

func TestGateAllowsOpenPaths(t *testing.T) {
	s, _ := seedServer(t)
	for _, p := range []string{"/enter", "/healthz", "/logout", "/auth/challenge"} {
		rec := httptest.NewRecorder()
		s.gate(s.mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code == http.StatusFound && rec.Header().Get("Location") == "/enter" && p != "/logout" {
			t.Errorf("open path %q was gated", p)
		}
	}
}

// castVote posts a ballot with the given csrf token and session identity.
func castVote(t *testing.T, s *Server, slug, identity, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"slug": {slug}, "vote": {"Yes"}, "rationale": {"Looks sound."}}
	if csrf != "" {
		form.Set("csrf", csrf)
	}
	r := httptest.NewRequest(http.MethodPost, "/vote", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if identity != "" {
		r.AddCookie(session(s, identity))
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)
	return rec
}

func TestCastVoteRequiresCSRF(t *testing.T) {
	s, act := seedServer(t)
	slug := fmt.Sprintf("%s-%d", act.TxHash, act.Index)

	if rec := castVote(t, s, slug, "Junia Marcia", ""); rec.Code != http.StatusForbidden {
		t.Errorf("vote with no CSRF token = %d, want 403", rec.Code)
	}
	if rec := castVote(t, s, slug, "Junia Marcia", "wrong-token"); rec.Code != http.StatusForbidden {
		t.Errorf("vote with a bad CSRF token = %d, want 403", rec.Code)
	}

	// A token minted for one delegate must not authorize a post as another.
	if rec := castVote(t, s, slug, "Junia Marcia", s.csrfToken("Titus Varo")); rec.Code != http.StatusForbidden {
		t.Errorf("vote with another delegate's CSRF token = %d, want 403", rec.Code)
	}

	// Nothing above should have been recorded.
	votes, err := s.db.MemberVotesFor(act.ProposalID)
	if err != nil {
		t.Fatalf("read member votes: %v", err)
	}
	if len(votes) != 0 {
		t.Errorf("a rejected vote was still recorded: %v", votes)
	}
}

func TestCastVoteWithCSRFSucceeds(t *testing.T) {
	s, act := seedServer(t)
	slug := fmt.Sprintf("%s-%d", act.TxHash, act.Index)

	rec := castVote(t, s, slug, "Junia Marcia", s.csrfToken("Junia Marcia"))
	if rec.Code != http.StatusFound {
		t.Fatalf("valid vote = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}

	votes, err := s.db.MemberVotesFor(act.ProposalID)
	if err != nil {
		t.Fatalf("read member votes: %v", err)
	}
	got, ok := votes["Junia Marcia"]
	if !ok {
		t.Fatalf("vote was not recorded; have %v", votes)
	}
	if got.Vote != "Yes" || got.Rationale != "Looks sound." {
		t.Errorf("recorded vote = %+v, want Yes / \"Looks sound.\"", got)
	}
}

func TestCastVoteWithoutSessionRedirects(t *testing.T) {
	s, act := seedServer(t)
	slug := fmt.Sprintf("%s-%d", act.TxHash, act.Index)

	rec := castVote(t, s, slug, "", s.csrfToken("Junia Marcia"))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/enter" {
		t.Errorf("vote with no session = %d -> %q; want 302 -> /enter", rec.Code, rec.Header().Get("Location"))
	}
}
