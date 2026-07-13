// Package config loads Cella's runtime configuration from the environment.
// Every value has a sensible default, so Cella runs with zero configuration.
package config

import "os"

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
