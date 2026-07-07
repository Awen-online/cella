// Package llm provides pluggable, "bring your own model" constitutionality
// review for Cella. The default provider speaks the OpenAI-compatible
// chat-completions API, so it works with OpenAI, OpenRouter, Together, Groq,
// vLLM, LM Studio, and a fully local Ollama — a committee brings its own model
// and can keep everything on its own infrastructure.
//
// The model is assistive only: it drafts an assessment; the committee decides
// and signs.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Assessment is a structured constitutionality-review result.
type Assessment struct {
	Verdict string // constitutional | unconstitutional | uncertain
	Summary string
	Model   string
}

// ActionInput is the governance action being assessed.
type ActionInput struct {
	Type     string
	Title    string
	Abstract string
}

// Provider assesses a governance action's constitutionality.
type Provider interface {
	Assess(ctx context.Context, in ActionInput) (Assessment, error)
	Model() string
}

const systemPrompt = `You are assisting a Cardano Constitutional Committee. Assess whether the described governance action aligns with the Cardano Constitution. You are an assistant only; the committee makes the final decision. Respond with ONLY a JSON object of the form {"verdict": "constitutional" | "unconstitutional" | "uncertain", "summary": "one to three sentences of reasoning"}. Use "uncertain" when there is not enough detail to judge.`

// OpenAICompatible calls any OpenAI-style /chat/completions endpoint.
type OpenAICompatible struct {
	baseURL string
	model   string
	key     string
	http    *http.Client
}

// NewOpenAICompatible returns a provider for baseURL (e.g.
// https://api.openai.com/v1, or http://localhost:11434/v1 for Ollama). key may
// be empty, e.g. for local models.
func NewOpenAICompatible(baseURL, model, key string) *OpenAICompatible {
	return &OpenAICompatible{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		key:     key,
		http:    &http.Client{Timeout: 90 * time.Second},
	}
}

// Model reports the configured model name.
func (p *OpenAICompatible) Model() string { return p.model }

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Assess sends the action to the model and returns its structured verdict.
func (p *OpenAICompatible) Assess(ctx context.Context, in ActionInput) (Assessment, error) {
	user := fmt.Sprintf("Governance action type: %s\nTitle: %s\nAbstract: %s",
		in.Type, orNone(in.Title), orNone(in.Abstract))

	reqBody, _ := json.Marshal(chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: user},
		},
		Temperature: 0.1,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return Assessment{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.key != "" {
		req.Header.Set("Authorization", "Bearer "+p.key)
	}

	resp, err := p.http.Do(req)
	if err != nil {
		return Assessment{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return Assessment{}, fmt.Errorf("llm %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return Assessment{}, fmt.Errorf("decode chat response: %w", err)
	}
	if cr.Error != nil {
		return Assessment{}, fmt.Errorf("llm error: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return Assessment{}, fmt.Errorf("llm returned no choices")
	}

	a := parseAssessment(cr.Choices[0].Message.Content)
	a.Model = p.model
	return a, nil
}

// parseAssessment extracts the {verdict, summary} JSON the model was asked to
// return, tolerating surrounding prose or code fences.
func parseAssessment(content string) Assessment {
	var raw struct {
		Verdict string `json:"verdict"`
		Summary string `json:"summary"`
	}
	_ = json.Unmarshal([]byte(extractJSONObject(content)), &raw)

	verdict := strings.ToLower(strings.TrimSpace(raw.Verdict))
	switch verdict {
	case "constitutional", "unconstitutional", "uncertain":
	default:
		verdict = "uncertain"
	}

	summary := strings.TrimSpace(raw.Summary)
	if summary == "" {
		summary = strings.TrimSpace(content)
	}
	return Assessment{Verdict: verdict, Summary: summary}
}

func extractJSONObject(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none provided)"
	}
	return s
}
