// Package config reads process configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrInvalid = errors.New("invalid config value")
	ErrMissing = errors.New("missing config value")
)

type Provider string

const (
	ProviderOpenAICompat Provider = "openai_compat"
	ProviderAnthropic    Provider = "anthropic"
	ProviderFake         Provider = "fake"
)

type LLM struct {
	Provider Provider
	BaseURL  string
	Model    string
	APIKey   string
	// ReasoningEffort is passed to the provider as is; empty keeps the model default.
	ReasoningEffort string
}

type Embeddings struct {
	Enabled bool
	Model   string
}

type Config struct {
	LLM           LLM
	Embeddings    Embeddings
	DatabaseURL   string
	RedisAddr     string
	MocksBaseURL  string
	SessionSecret string
	MockFailRate  float64
	HTTPAddr      string
	GRPCAddr      string
	// FeatureRequests turns on the access requests module.
	FeatureRequests bool
}

// Load builds Config from lookup (os.LookupEnv in production).
// Unset variables get defaults; set but malformed ones fail with ErrInvalid.
func Load(lookup func(string) (string, bool)) (Config, error) {
	str := func(key, def string) string {
		if v, ok := lookup(key); ok {
			return v
		}
		return def
	}

	cfg := Config{
		LLM: LLM{
			Provider:        Provider(str("LLM_PROVIDER", string(ProviderOpenAICompat))),
			BaseURL:         str("LLM_BASE_URL", ""),
			Model:           str("LLM_MODEL", ""),
			APIKey:          str("LLM_API_KEY", ""),
			ReasoningEffort: str("LLM_REASONING_EFFORT", ""),
		},
		Embeddings:    Embeddings{Model: str("EMBEDDINGS_MODEL", "")},
		DatabaseURL:   str("DATABASE_URL", ""),
		RedisAddr:     str("REDIS_ADDR", ""),
		MocksBaseURL:  str("MOCKS_BASE_URL", ""),
		SessionSecret: str("SESSION_SECRET", ""),
		HTTPAddr:      str("HTTP_ADDR", ":8080"),
		GRPCAddr:      str("GRPC_ADDR", ":9090"),
	}

	switch cfg.LLM.Provider {
	case ProviderOpenAICompat, ProviderAnthropic, ProviderFake:
	default:
		return Config{}, fmt.Errorf("LLM_PROVIDER=%q: %w", cfg.LLM.Provider, ErrInvalid)
	}

	switch cfg.LLM.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high":
	default:
		return Config{}, fmt.Errorf("LLM_REASONING_EFFORT=%q: %w", cfg.LLM.ReasoningEffort, ErrInvalid)
	}

	var err error
	if cfg.Embeddings.Enabled, err = strconv.ParseBool(str("EMBEDDINGS_ENABLED", "false")); err != nil {
		return Config{}, fmt.Errorf("EMBEDDINGS_ENABLED: %w", ErrInvalid)
	}

	if cfg.FeatureRequests, err = strconv.ParseBool(str("FEATURE_REQUESTS", "true")); err != nil {
		return Config{}, fmt.Errorf("FEATURE_REQUESTS: %w", ErrInvalid)
	}

	if cfg.MockFailRate, err = strconv.ParseFloat(str("MOCK_FAIL_RATE", "0"), 64); err != nil {
		return Config{}, fmt.Errorf("MOCK_FAIL_RATE: %w", ErrInvalid)
	}
	if cfg.MockFailRate < 0 || cfg.MockFailRate > 1 {
		return Config{}, fmt.Errorf("MOCK_FAIL_RATE=%v outside [0, 1]: %w", cfg.MockFailRate, ErrInvalid)
	}

	return cfg, nil
}

// RequireDatabase is called by commands that need Postgres.
func (c Config) RequireDatabase() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL: %w", ErrMissing)
	}
	return nil
}

// RequireRedis is called by commands that queue jobs or stream events.
func (c Config) RequireRedis() error {
	if c.RedisAddr == "" {
		return fmt.Errorf("REDIS_ADDR: %w", ErrMissing)
	}
	return nil
}
