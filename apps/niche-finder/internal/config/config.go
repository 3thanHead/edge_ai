// Package config loads all runtime settings from the environment, matching the
// monorepo convention that each app is configured entirely through its .env.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config is the fully-resolved runtime configuration.
type Config struct {
	// LLM cluster (HAProxy endpoint, Ollama native API — same as every app).
	LLMBaseURL string
	LLMModel   string

	// SeedCategories are high-level categories the user wants niches within, e.g.
	// ["Cookbooks","Printable planners"]. Empty => fully autonomous discovery from
	// the Reddit/Trends leads. The UI can override these at runtime.
	SeedCategories []string

	// Mining. NicheSources is the raw adapter spec, e.g.
	//   "reddit:somethingimade,Etsy,crafts;gtrends:"
	// parsed by the source registry. EtsyAPIKey, when set, enables the Etsy
	// adapter (mining + measured saturation) via the official Open API v3.
	NicheSources string
	EtsyAPIKey   string
	MineLimit    int // items pulled per source

	// SaturationMarkets is the order in which measured-saturation backends are
	// tried, e.g. ["etsy","ebay"]; the first that yields a real count wins,
	// otherwise the scorer falls back to an LLM estimate.
	SaturationMarkets []string

	// Server + storage.
	Addr    string // internal listen address, e.g. ":8820" (compose maps the host port)
	DataDir string // bbolt DB lives here
}

// Load reads the environment and applies sensible defaults.
func Load() Config {
	return Config{
		LLMBaseURL: env("LLM_BASE_URL", "http://localhost:11434"),
		LLMModel:   env("LLM_MODEL", "qwen2.5:14b"),

		SeedCategories: envList("SEED_CATEGORIES", nil),

		NicheSources: env("NICHE_SOURCES", "reddit:somethingimade,Etsy,crafts;gtrends:"),
		EtsyAPIKey:   env("ETSY_API_KEY", ""),
		MineLimit:    envInt("MINE_LIMIT", 25),

		SaturationMarkets: envList("SATURATION_MARKETS", []string{"etsy", "ebay"}),

		Addr:    env("LISTEN_ADDR", ":8820"),
		DataDir: env("DATA_DIR", "/data"),
	}
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// envList reads a comma-separated list, trimming blanks; empty => def.
func envList(k string, def []string) []string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
