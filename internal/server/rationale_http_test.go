package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Awen-online/cella/internal/rationale"
	"github.com/Awen-online/cella/internal/store"
)

// authored is a complete, anchorable rationale.
var authored = store.Rationale{
	Summary:    "The committee finds the withdrawal constitutional.",
	Statement:  "The request is proportionate and within the treasury guardrails.",
	Conclusion: "Approved.",
	AuthoredBy: "Junia Marcia",
}

func slugOf(s *Server, t *testing.T) (string, string) {
	t.Helper()
	acts, err := s.db.Actions(1)
	if err != nil || len(acts) == 0 {
		t.Fatalf("no seeded action: %v", err)
	}
	return acts[0].Slug(), acts[0].ProposalID
}

func getAs(t *testing.T, s *Server, path, identity string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if identity != "" {
		r.AddCookie(session(s, identity))
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)
	return rec
}

func TestRationalePageRenders(t *testing.T) {
	s, _ := seedServer(t)
	slug, _ := slugOf(s, t)

	rec := getAs(t, s, "/rationale/"+slug, "Junia Marcia")
	if rec.Code != http.StatusOK {
		t.Fatalf("rationale page = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Author the rationale", "Summary", "Rationale statement", "internalVote"} {
		if !strings.Contains(body, want) {
			t.Errorf("rationale page missing %q", want)
		}
	}
	// Nothing authored yet, so it must say so rather than offering an anchor.
	if !strings.Contains(body, "Not ready to anchor") {
		t.Error("an unauthored rationale did not report itself unready to anchor")
	}
}

func TestSaveRationale(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	form := url.Values{
		"summary":    {authored.Summary},
		"statement":  {authored.Statement},
		"conclusion": {authored.Conclusion},
		"csrf":       {s.csrfToken("Junia Marcia")},
	}
	r := httptest.NewRequest(http.MethodPost, "/rationale/"+slug, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(session(s, "Junia Marcia"))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)

	if rec.Code != http.StatusFound {
		t.Fatalf("save = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}
	got, ok, err := s.db.RationaleFor(pid)
	if err != nil || !ok {
		t.Fatalf("rationale was not stored (ok=%v err=%v)", ok, err)
	}
	if got.Summary != authored.Summary || got.Statement != authored.Statement {
		t.Errorf("stored rationale = %+v, want summary/statement to match", got)
	}
	if got.AuthoredBy != "Junia Marcia" {
		t.Errorf("AuthoredBy = %q, want \"Junia Marcia\"", got.AuthoredBy)
	}
}

func TestSaveRationaleRequiresCSRF(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	form := url.Values{"summary": {"s"}, "statement": {"r"}} // no csrf
	r := httptest.NewRequest(http.MethodPost, "/rationale/"+slug, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(session(s, "Junia Marcia"))
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, r)

	if rec.Code != http.StatusForbidden {
		t.Errorf("save with no CSRF token = %d, want 403", rec.Code)
	}
	if _, ok, _ := s.db.RationaleFor(pid); ok {
		t.Error("a rationale was stored despite the CSRF check failing")
	}
}

// The download is the artifact, so it must be a valid CIP-136 document whose
// hash is reproducible from the bytes served.
func TestDownloadJSONLD(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	if err := s.db.UpsertRationale(pid, authored); err != nil {
		t.Fatalf("seed rationale: %v", err)
	}
	// Two delegates take a position, so internalVote has something real in it.
	if err := s.db.UpsertMemberVote(pid, "Junia Marcia", "Yes", "Sound.", "", ""); err != nil {
		t.Fatalf("seed vote: %v", err)
	}
	if err := s.db.UpsertMemberVote(pid, "Titus Varo", "No", "Overreach.", "", ""); err != nil {
		t.Fatalf("seed vote: %v", err)
	}

	rec := getAs(t, s, "/rationale/"+slug+".jsonld", "Junia Marcia")
	if rec.Code != http.StatusOK {
		t.Fatalf("download = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/ld+json" {
		t.Errorf("Content-Type = %q, want application/ld+json", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, ".jsonld") {
		t.Errorf("Content-Disposition = %q, want an attachment filename", cd)
	}

	served := rec.Body.Bytes()

	var doc map[string]any
	if err := json.Unmarshal(served, &doc); err != nil {
		t.Fatalf("served document is not valid JSON: %v", err)
	}
	for _, k := range []string{"@context", "hashAlgorithm", "body", "authors"} {
		if _, ok := doc[k]; !ok {
			t.Errorf("served document is missing %q", k)
		}
	}

	body := doc["body"].(map[string]any)
	if body["summary"] != authored.Summary {
		t.Errorf("summary = %v, want %q", body["summary"], authored.Summary)
	}

	// The internal split must reflect the delegates' actual positions: 1 Yes,
	// 1 No, and the rest of the roster not having voted.
	iv := body["internalVote"].(map[string]any)
	if iv["constitutional"] != float64(1) || iv["unconstitutional"] != float64(1) || iv["abstain"] != float64(0) {
		t.Errorf("internalVote = %v, want 1 constitutional / 1 unconstitutional / 0 abstain", iv)
	}
	if want := float64(len(testBody.Members) - 2); iv["didNotVote"] != want {
		t.Errorf("didNotVote = %v, want %v (roster members with no recorded position)", iv["didNotVote"], want)
	}

	// Both voting delegates author the document; the silent ones do not.
	authors := doc["authors"].([]any)
	if len(authors) != 2 {
		t.Fatalf("authors = %d entries, want 2 (the delegates who voted)", len(authors))
	}

	// The page must advertise exactly the hash of the bytes it just served —
	// this is the value that goes on-chain.
	page := getAs(t, s, "/rationale/"+slug, "Junia Marcia").Body.String()
	if want := rationale.AnchorHash(served); !strings.Contains(page, want) {
		t.Errorf("page does not show the anchor hash %s of the document it serves", want)
	}
}

// A half-written rationale must not yield a downloadable file: an anchor hash
// over an invalid document looks submittable but is not.
func TestDownloadRefusesIncompleteRationale(t *testing.T) {
	s, _ := seedServer(t)
	slug, pid := slugOf(s, t)

	// Summary but no rationale statement — CIP-136 requires both.
	if err := s.db.UpsertRationale(pid, store.Rationale{Summary: "Constitutional."}); err != nil {
		t.Fatalf("seed rationale: %v", err)
	}

	rec := getAs(t, s, "/rationale/"+slug+".jsonld", "Junia Marcia")
	if rec.Code != http.StatusConflict {
		t.Errorf("download of an incomplete rationale = %d, want 409", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "@context") {
		t.Error("an incomplete rationale was served as a document anyway")
	}
}

func TestRationaleUnknownAction(t *testing.T) {
	s, _ := seedServer(t)
	for _, p := range []string{"/rationale/nonexistent-9", "/rationale/nonexistent-9.jsonld"} {
		if rec := getAs(t, s, p, "Junia Marcia"); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", p, rec.Code)
		}
	}
}

// The decision the committee submits is the plurality of the delegates who
// actually voted — and a tie is not a mandate.
func TestTallyDecision(t *testing.T) {
	cases := []struct {
		name string
		t    tally
		want string
	}{
		{"clear yes", tally{Yes: 3, No: 1}, "Yes"},
		{"clear no", tally{Yes: 1, No: 3}, "No"},
		{"tied yes/no abstains", tally{Yes: 2, No: 2}, "Abstain"},
		{"abstain outweighs", tally{Yes: 1, No: 1, Abstain: 3}, "Abstain"},
		{"nobody voted", tally{DidNotVote: 5}, "Abstain"},
		{"yes ties abstain but beats no", tally{Yes: 2, No: 1, Abstain: 2}, "Yes"},
		{"yes beaten by abstain", tally{Yes: 2, No: 1, Abstain: 3}, "Abstain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.t.Decision(); got != tc.want {
				t.Errorf("tally%+v.Decision() = %q, want %q", tc.t, got, tc.want)
			}
		})
	}
}

func TestTallyForCountsRoster(t *testing.T) {
	s, _ := seedServer(t)
	_, pid := slugOf(s, t)

	if err := s.db.UpsertMemberVote(pid, "Junia Marcia", "Yes", "", "", ""); err != nil {
		t.Fatalf("seed vote: %v", err)
	}
	if err := s.db.UpsertMemberVote(pid, "Cullah", "Abstain", "", "", ""); err != nil {
		t.Fatalf("seed vote: %v", err)
	}

	got, authors, err := s.tallyFor(pid)
	if err != nil {
		t.Fatalf("tallyFor: %v", err)
	}
	want := tally{Yes: 1, Abstain: 1, DidNotVote: len(testBody.Members) - 2}
	if got != want {
		t.Errorf("tallyFor = %+v, want %+v", got, want)
	}
	if len(authors) != 2 {
		t.Errorf("authors = %v, want the 2 delegates who voted", authors)
	}
	if fmt.Sprint(got.Recorded()) != "2" {
		t.Errorf("Recorded() = %d, want 2", got.Recorded())
	}
}
