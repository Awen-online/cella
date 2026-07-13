package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func postForm(t *testing.T, s *Server, path, identity string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	form.Set("csrf", s.csrfToken(identity))
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(session(s, identity))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)
	return rec
}

func TestFlagToggles(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	// Raise.
	if rec := postForm(t, s, "/flag", "Junia Marcia", url.Values{"slug": {slug}, "flag": {"discuss"}}); rec.Code != http.StatusFound {
		t.Fatalf("raise = %d, want 302", rec.Code)
	}
	flags, err := s.db.FlagsFor(pid)
	if err != nil {
		t.Fatalf("FlagsFor: %v", err)
	}
	if len(flags["discuss"]) != 1 || flags["discuss"][0].Member != "Junia Marcia" {
		t.Fatalf("flags = %v, want Junia Marcia on discuss", flags)
	}

	// The same delegate toggles it back down.
	if rec := postForm(t, s, "/flag", "Junia Marcia", url.Values{"slug": {slug}, "flag": {"discuss"}}); rec.Code != http.StatusFound {
		t.Fatalf("lower = %d, want 302", rec.Code)
	}
	flags, _ = s.db.FlagsFor(pid)
	if len(flags["discuss"]) != 0 {
		t.Errorf("the delegate's own flag did not come down: %v", flags)
	}
}

// A flag is a delegate raising a concern to the chamber. One delegate clicking
// the same flag must not lower another's — that would silence a colleague, which
// is the opposite of what a flag is for. (The WordPress deployment this replaces
// stores one holder per flag, so a click overwrites whoever raised it.)
func TestOneDelegateCannotLowerAnothersFlag(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	postForm(t, s, "/flag", "Junia Marcia", url.Values{"slug": {slug}, "flag": {"discuss"}})
	postForm(t, s, "/flag", "Titus Varo", url.Values{"slug": {slug}, "flag": {"discuss"}})

	flags, err := s.db.FlagsFor(pid)
	if err != nil {
		t.Fatalf("FlagsFor: %v", err)
	}
	if len(flags["discuss"]) != 2 {
		t.Fatalf("both delegates should hold the flag; got %v", flags["discuss"])
	}

	// Junia lowers hers. Titus's must stand.
	postForm(t, s, "/flag", "Junia Marcia", url.Values{"slug": {slug}, "flag": {"discuss"}})

	flags, _ = s.db.FlagsFor(pid)
	if len(flags["discuss"]) != 1 || flags["discuss"][0].Member != "Titus Varo" {
		t.Errorf("Titus's flag was lowered by Junia's toggle: %v", flags["discuss"])
	}
}

func TestFlagRequiresCSRFAndAValidKind(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	// No CSRF token.
	form := url.Values{"slug": {slug}, "flag": {"discuss"}}
	r := httptest.NewRequest(http.MethodPost, "/flag", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(session(s, "Junia Marcia"))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("flag with no CSRF token = %d, want 403", rec.Code)
	}

	// An invented flag kind.
	if rec := postForm(t, s, "/flag", "Junia Marcia", url.Values{"slug": {slug}, "flag": {"sabotage"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown flag kind = %d, want 400", rec.Code)
	}

	if flags, _ := s.db.FlagsFor(pid); len(flags) != 0 {
		t.Errorf("a rejected flag was raised anyway: %v", flags)
	}
}

func TestDraftSavesAndIsPrivate(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	const note = "The reporting cadence is looser than Article IV seems to require. Check precedent."
	if rec := postForm(t, s, "/draft", "Junia Marcia", url.Values{"slug": {slug}, "body": {note}}); rec.Code != http.StatusOK {
		t.Fatalf("save draft = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	got, at, err := s.db.Draft(pid, "Junia Marcia")
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if got != note {
		t.Errorf("draft = %q, want %q", got, note)
	}
	if at == 0 {
		t.Error("the draft was not timestamped")
	}

	// Nobody else's notes are reachable — not by any request, because the query
	// is keyed on the reader's own identity.
	other, _, err := s.db.Draft(pid, "Titus Varo")
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if other != "" {
		t.Errorf("another delegate's notes leaked: %q", other)
	}

	// And the page a colleague sees must not contain them.
	page := getAs(t, s, "/action/"+slug, "Titus Varo").Body.String()
	if strings.Contains(page, note) {
		t.Error("one delegate's private notes were rendered on another delegate's page")
	}
	// The author's own page does show them.
	mine := getAs(t, s, "/action/"+slug, "Junia Marcia").Body.String()
	if !strings.Contains(mine, note) {
		t.Error("a delegate's own notes are missing from their own page")
	}
}

func TestDraftRequiresCSRF(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	form := url.Values{"slug": {slug}, "body": {"secret"}}
	r := httptest.NewRequest(http.MethodPost, "/draft", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(session(s, "Junia Marcia"))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Errorf("draft with no CSRF token = %d, want 403", rec.Code)
	}
	if got, _, _ := s.db.Draft(pid, "Junia Marcia"); got != "" {
		t.Error("a draft was stored despite the CSRF check failing")
	}
}

// An autosaving textarea must not be able to fill the disk.
func TestDraftIsBounded(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	huge := strings.Repeat("x", draftLimit+1)
	rec := postForm(t, s, "/draft", "Junia Marcia", url.Values{"slug": {slug}, "body": {huge}})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized draft = %d, want 413", rec.Code)
	}
	if got, _, _ := s.db.Draft(pid, "Junia Marcia"); got != "" {
		t.Error("an oversized draft was stored")
	}
}
