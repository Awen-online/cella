package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	p := NewOpenAICompatible(srv.URL, "test-model", "test-key", "")
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

// TestAssessGroundsReviewInConstitution verifies that when a Constitution is
// configured, its text is sent to the model in the system prompt (alongside the
// base instruction) — i.e. the review is grounded in the actual document rather
// than the model's training memory.
func TestAssessGroundsReviewInConstitution(t *testing.T) {
	const marker = "ARTICLE-SENTINEL-9F3: the treasury shall not be raided"

	var gotSystem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		for _, m := range req.Messages {
			if m.Role == "system" {
				gotSystem = m.Content
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": `{"verdict":"uncertain","summary":"n/a"}`}},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAICompatible(srv.URL, "test-model", "", marker)
	if _, err := p.Assess(context.Background(), ActionInput{Type: "InfoAction", Title: "t"}); err != nil {
		t.Fatalf("Assess: %v", err)
	}

	if !strings.Contains(gotSystem, marker) {
		t.Errorf("system prompt did not include the Constitution text; got:\n%s", gotSystem)
	}
	if !strings.Contains(gotSystem, systemPrompt) {
		t.Error("system prompt dropped the base instruction")
	}
	if !strings.Contains(gotSystem, "CARDANO CONSTITUTION") {
		t.Error("system prompt missing the Constitution delimiter/heading")
	}
}

// TestSystemMessageWithoutConstitution verifies that with no Constitution
// configured, the system prompt is exactly the base instruction (no grounding
// block appended).
func TestSystemMessageWithoutConstitution(t *testing.T) {
	p := NewOpenAICompatible("http://example.invalid", "m", "", "")
	if got := p.systemMessage(); got != systemPrompt {
		t.Errorf("expected bare system prompt, got:\n%s", got)
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
