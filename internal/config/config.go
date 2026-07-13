// Package config loads Cella's runtime configuration from the environment.
// Every value has a sensible default, so Cella runs with zero configuration.
package config

import (
	"os"
	"strings"

	"github.com/Awen-online/cella/internal/network"
)

// Config holds Cella's runtime settings.
type Config struct {
	DBPath     string // path to the SQLite database file
	Addr       string // web server listen address
	KoiosToken string // optional Koios bearer token

	// Network is the Cardano network this instance runs against: mainnet,
	// preprod or preview. It picks the Koios endpoint and the explorer links,
	// so a committee practising on a testnet is on that testnet everywhere —
	// not merely in the data, with its explorer links still pointing at mainnet.
	Network network.Network

	// KoiosOverride replaces the endpoint Network would choose — for a private or
	// self-hosted Koios. Empty means "follow the network", which is what lets a
	// --network flag change the endpoint without the caller restating it.
	KoiosOverride string

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

	// BodyPath points at the JSON file describing who this Cella instance
	// belongs to: the consortium or single member holding the Constitutional
	// Committee seat, their logo (a file alongside it), and their members —
	// names, handles, wallet addresses (how Cella recognises them at sign-in),
	// and on-chain voting key hashes.
	//
	// Without it Cella serves a placeholder that is deliberately nobody. An
	// instance that quietly impersonated a consortium it is not would be worse
	// than one that admits it has not been told who it is.
	BodyPath string

	// HotNFTAddr is the script address holding the committee's hot NFT. Its
	// inline datum names the voting group — who may sign the committee's vote —
	// and therefore what quorum actually is. Cella reads it from the chain
	// rather than trusting local configuration, because the validator, not
	// Cella, decides whether a vote transaction is accepted.
	HotNFTAddr string

	// Constitutionality review — bring your own model. Any OpenAI-compatible
	// endpoint (OpenAI, OpenRouter, Groq, vLLM, LM Studio, local Ollama).
	LLMURL   string // e.g. https://api.openai.com/v1 or http://localhost:11434/v1
	LLMModel string // e.g. gpt-4o-mini or llama3.1
	LLMKey   string // optional (local models need none)
}

// Load reads configuration from the environment, applying defaults. An
// unrecognised network is an error rather than a silent fall back to mainnet:
// an operator who meant to practise on a testnet and quietly ended up on
// mainnet would be voting for real.
func Load() (Config, error) {
	net, err := network.Parse(os.Getenv("CELLA_NETWORK"))
	if err != nil {
		return Config{}, err
	}

	c := Config{
		Network:    net,
		DBPath:     env("CELLA_DB", "cella.db"),
		Addr:       env("CELLA_ADDR", ":8080"),
		KoiosToken: os.Getenv("KOIOS_TOKEN"),
		Secret:     os.Getenv("CELLA_SECRET"),
		Demo:       truthy(os.Getenv("CELLA_DEMO")),
		BodyPath:   env("CELLA_BODY", os.Getenv("CELLA_ROSTER")), // CELLA_ROSTER: the old name
		HotNFTAddr: os.Getenv("CELLA_HOT_NFT_ADDR"),
		LLMURL:     os.Getenv("CELLA_LLM_URL"),
		LLMModel:   os.Getenv("CELLA_LLM_MODEL"),
		LLMKey:     os.Getenv("CELLA_LLM_KEY"),
	}

	c.KoiosOverride = os.Getenv("KOIOS_URL")
	return c, nil
}

// Koios is the endpoint to talk to: the override when one was given, otherwise
// whatever the network implies. Derived rather than stored, so changing the
// network — from a flag, say — moves the endpoint with it.
func (c Config) Koios() string {
	if c.KoiosOverride != "" {
		return c.KoiosOverride
	}
	return c.Network.KoiosURL()
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
