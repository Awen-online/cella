package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestOpenAICompatibleAssess spins up a fake OpenAI-compatible endpoint and
// verifies the full request/response path, including tolerating a code-fenced
// JSON body and normalizing the verdict.
func TestOpenAICompatibleAssess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %s, want /chat/completions", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/incorrect auth header: %q", r.Header.Get("Authorization"))
		}
		// A realistic model reply: JSON wrapped in a Markdown code fence, with a
		// capitalized verdict we expect to be normalized.
		content := "```json\n{\"verdict\":\"Constitutional\",\"summary\":\"Aligns with the treasury guardrails.\"}\n```"
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": content}},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAICompatible(srv.URL, "test-model", "test-key")
	a, err := p.Assess(context.Background(), ActionInput{Type: "TreasuryWithdrawals", Title: "Fund a public good"})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if a.Verdict != "constitutional" {
		t.Errorf("verdict = %q, want constitutional", a.Verdict)
	}
	if a.Summary == "" {
		t.Error("empty summary")
	}
	if a.Model != "test-model" {
		t.Errorf("model = %q, want test-model", a.Model)
	}
}

func TestParseAssessmentFallsBackToUncertain(t *testing.T) {
	a := parseAssessment("the model rambled with no json at all")
	if a.Verdict != "uncertain" {
		t.Errorf("verdict = %q, want uncertain", a.Verdict)
	}
	if a.Summary == "" {
		t.Error("summary should fall back to raw content")
	}
}
