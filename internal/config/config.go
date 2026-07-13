// Package config loads Cella's runtime configuration from the environment.
// Every value has a sensible default, so Cella runs with zero configuration.
package config

import (
	"os"
	"strings"
)

// Config holds Cella's runtime settings.
type Config struct {
	DBPath     string // path to the SQLite database file
	Addr       string // web server listen address
	KoiosURL   string // Koios API base URL
	KoiosToken string // optional Koios bearer token

	// Secret keys the session cookies. When empty a random key is generated at
	// startup, which is secure but ephemeral: sessions do not survive a restart.
	// Set it for any persistent deployment.
	Secret string

	// Demo enables the roster picker on the entry splash, which signs a visitor
	// in as any delegate they choose with no proof of identity whatsoever. It
	// exists to demonstrate the chamber without wallets. A real deployment must
	// never set it: anyone reachable could vote as any delegate and author the
	// committee's rationale.
	Demo bool

	// RosterPath points at a JSON file describing the body's delegates: their
	// names, roles, wallet addresses (how Cella recognises them at sign-in) and
	// on-chain voting key hashes. Without it Cella uses a placeholder roster,
	// whose addresses are not real and so can authenticate nobody.
	RosterPath string

	// HotNFTAddr is the script address holding the committee's hot NFT. Its
	// inline datum names the voting group — who may sign the committee's vote —
	// and therefore what quorum actually is. Cella reads it from the chain
	// rather than trusting local configuration, because the validator, not
	// Cella, decides whether a vote transaction is accepted.
	HotNFTAddr string

	// LogoPath is a file on disk — the body's own mark. Cella serves it itself
	// rather than letting the page hot-link it: the Content-Security-Policy is
	// img-src 'self', and a governance tool should not go dark because someone
	// else's server did.
	LogoPath string

	// Constitutionality review — bring your own model. Any OpenAI-compatible
	// endpoint (OpenAI, OpenRouter, Groq, vLLM, LM Studio, local Ollama).
	LLMURL   string // e.g. https://api.openai.com/v1 or http://localhost:11434/v1
	LLMModel string // e.g. gpt-4o-mini or llama3.1
	LLMKey   string // optional (local models need none)
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		DBPath:     env("CELLA_DB", "cella.db"),
		Addr:       env("CELLA_ADDR", ":8080"),
		KoiosURL:   env("KOIOS_URL", "https://api.koios.rest/api/v1"),
		KoiosToken: os.Getenv("KOIOS_TOKEN"),
		Secret:     os.Getenv("CELLA_SECRET"),
		Demo:       truthy(os.Getenv("CELLA_DEMO")),
		RosterPath: os.Getenv("CELLA_ROSTER"),
		HotNFTAddr: os.Getenv("CELLA_HOT_NFT_ADDR"),
		LogoPath:   os.Getenv("CELLA_LOGO"),
		LLMURL:     os.Getenv("CELLA_LLM_URL"),
		LLMModel:   os.Getenv("CELLA_LLM_MODEL"),
		LLMKey:     os.Getenv("CELLA_LLM_KEY"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// truthy reads a boolean-ish environment value. Anything unrecognised is false:
// a setting that weakens authentication must be opted into deliberately, not
// tripped by a typo.
func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
