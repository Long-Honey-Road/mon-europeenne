package config

import (
	"errors"
	"testing"
)

func envOf(vars map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := Load(envOf(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.LLM.Provider != ProviderOpenAICompat {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, ProviderOpenAICompat)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.GRPCAddr != ":9090" {
		t.Errorf("GRPCAddr = %q, want :9090", cfg.GRPCAddr)
	}
	if cfg.Embeddings.Enabled {
		t.Error("Embeddings.Enabled = true, want false by default")
	}
	if cfg.MockFailRate != 0 {
		t.Errorf("MockFailRate = %v, want 0", cfg.MockFailRate)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"LLM_PROVIDER":         "fake",
		"LLM_BASE_URL":         "http://ollama:11434/v1",
		"LLM_MODEL":            "qwen3.6:35b-a3b",
		"LLM_API_KEY":          "secret",
		"LLM_REASONING_EFFORT": "none",
		"EMBEDDINGS_ENABLED":   "true",
		"EMBEDDINGS_MODEL":     "bge-m3",
		"DATABASE_URL":         "postgres://buddy:buddy@postgres:5432/buddy",
		"REDIS_ADDR":           "redis:6379",
		"MOCKS_BASE_URL":       "http://mocks:8090",
		"SESSION_SECRET":       "s3",
		"MOCK_FAIL_RATE":       "0.25",
		"HTTP_ADDR":            ":18080",
		"GRPC_ADDR":            ":19090",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := Config{
		LLM: LLM{
			Provider:        ProviderFake,
			BaseURL:         "http://ollama:11434/v1",
			Model:           "qwen3.6:35b-a3b",
			APIKey:          "secret",
			ReasoningEffort: "none",
		},
		Embeddings:    Embeddings{Enabled: true, Model: "bge-m3"},
		DatabaseURL:   "postgres://buddy:buddy@postgres:5432/buddy",
		RedisAddr:     "redis:6379",
		MocksBaseURL:  "http://mocks:8090",
		SessionSecret: "s3",
		MockFailRate:  0.25,
		HTTPAddr:      ":18080",
		GRPCAddr:      ":19090",
	}
	if cfg != want {
		t.Fatalf("Load() =\n%+v\nwant\n%+v", cfg, want)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"unknown provider":         {"LLM_PROVIDER": "gpt"},
		"bad bool":                 {"EMBEDDINGS_ENABLED": "yes please"},
		"bad float":                {"MOCK_FAIL_RATE": "often"},
		"fail rate above one":      {"MOCK_FAIL_RATE": "1.5"},
		"negative fail rate":       {"MOCK_FAIL_RATE": "-0.1"},
		"empty provider value":     {"LLM_PROVIDER": ""},
		"unknown reasoning effort": {"LLM_REASONING_EFFORT": "max"},
	}
	for name, vars := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(envOf(vars))
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Load() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestRequireDatabase(t *testing.T) {
	if err := (Config{}).RequireDatabase(); !errors.Is(err, ErrMissing) {
		t.Fatalf("RequireDatabase() on empty config = %v, want ErrMissing", err)
	}
	if err := (Config{DatabaseURL: "postgres://x"}).RequireDatabase(); err != nil {
		t.Fatalf("RequireDatabase() = %v, want nil", err)
	}
}

func TestRequireRedis(t *testing.T) {
	if err := (Config{}).RequireRedis(); !errors.Is(err, ErrMissing) {
		t.Fatalf("RequireRedis() on empty config = %v, want ErrMissing", err)
	}
	if err := (Config{RedisAddr: "redis:6379"}).RequireRedis(); err != nil {
		t.Fatalf("RequireRedis() = %v, want nil", err)
	}
}
