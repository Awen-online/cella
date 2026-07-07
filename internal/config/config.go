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
}

// Load reads configuration from the environment, applying defaults.
func Load() Config {
	return Config{
		DBPath:     env("CELLA_DB", "cella.db"),
		Addr:       env("CELLA_ADDR", ":8080"),
		KoiosURL:   env("KOIOS_URL", "https://api.koios.rest/api/v1"),
		KoiosToken: os.Getenv("KOIOS_TOKEN"),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
