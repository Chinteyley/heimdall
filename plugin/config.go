package heimdall

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config is the plugin's "config" object in Bifrost's config.json.
// String values written as "env.NAME" are read from the environment.
type Config struct {
	// TavilyAPIKey authenticates live searches. Required.
	TavilyAPIKey string `json:"tavily_api_key"`
	// TavilyURL is the Tavily API base, overridable for tests.
	TavilyURL string `json:"tavily_url"`
	// GatewayURL is where Heimdall sends its own model calls: this same Bifrost,
	// so the checks get its routing, fallbacks, caching and budgets.
	GatewayURL string `json:"gateway_url"`
	// GatewayHeaders are added to Heimdall's own model calls (a virtual key, say).
	GatewayHeaders map[string]string `json:"gateway_headers"`
	// CheckModel is the Bifrost model ("provider/model") that extracts and judges claims.
	CheckModel string `json:"check_model"`
	// DefaultMode applies when a request carries no x-heimdall header.
	DefaultMode Mode `json:"default_mode"`
	// MaxClaims caps how many claims one answer is checked for.
	MaxClaims int `json:"max_claims"`
	// MaxResults is the number of Tavily results per claim.
	MaxResults int `json:"max_results"`
	// SearchDepth is Tavily's search_depth: basic (1 credit) or advanced (2).
	SearchDepth string `json:"search_depth"`
	// TimeoutSeconds bounds one whole check; past it the answer passes through
	// with the report's error set.
	TimeoutSeconds int `json:"timeout_seconds"`
	// Paths are the request paths Heimdall acts on.
	Paths []string `json:"paths"`
}

func (c Config) timeout() time.Duration { return time.Duration(c.TimeoutSeconds) * time.Second }

func defaultConfig() Config {
	return Config{
		TavilyURL:      "https://api.tavily.com",
		GatewayURL:     "http://127.0.0.1:8080",
		CheckModel:     "openai/gpt-5.4-mini",
		DefaultMode:    ModeOff,
		MaxClaims:      6,
		MaxResults:     5,
		SearchDepth:    "basic",
		TimeoutSeconds: 45,
		Paths:          []string{"/v1/chat/completions", "/openai/v1/chat/completions"},
	}
}

// parseConfig reads the raw value Bifrost passes to Init.
func parseConfig(raw any) (Config, error) {
	cfg := defaultConfig()
	if raw != nil {
		b, err := json.Marshal(raw)
		if err != nil {
			return cfg, fmt.Errorf("heimdall: config: %w", err)
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("heimdall: config: %w", err)
		}
	}
	cfg.TavilyAPIKey = resolveEnv(cfg.TavilyAPIKey)
	if cfg.TavilyAPIKey == "" {
		cfg.TavilyAPIKey = os.Getenv("TAVILY_API_KEY")
	}
	for k, v := range cfg.GatewayHeaders {
		cfg.GatewayHeaders[k] = resolveEnv(v)
	}
	if cfg.TavilyAPIKey == "" {
		return cfg, fmt.Errorf("heimdall: tavily_api_key is required (or set TAVILY_API_KEY)")
	}
	if _, ok := parseMode(string(cfg.DefaultMode)); !ok {
		return cfg, fmt.Errorf("heimdall: default_mode %q is not one of off, report, annotate, correct", cfg.DefaultMode)
	}
	if cfg.DefaultMode == ModeCheck {
		return cfg, fmt.Errorf("heimdall: default_mode cannot be check")
	}
	if cfg.SearchDepth != "basic" && cfg.SearchDepth != "advanced" {
		return cfg, fmt.Errorf("heimdall: search_depth must be basic or advanced")
	}
	if cfg.MaxClaims < 1 || cfg.MaxResults < 1 || cfg.TimeoutSeconds < 1 {
		return cfg, fmt.Errorf("heimdall: max_claims, max_results and timeout_seconds must be positive")
	}
	cfg.GatewayURL = strings.TrimRight(cfg.GatewayURL, "/")
	cfg.TavilyURL = strings.TrimRight(cfg.TavilyURL, "/")
	return cfg, nil
}

func resolveEnv(v string) string {
	if name, ok := strings.CutPrefix(v, "env."); ok {
		return os.Getenv(name)
	}
	return v
}
