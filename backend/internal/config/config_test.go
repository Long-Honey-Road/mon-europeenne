package config

import (
	"errors"
	"reflect"
	"testing"
	"time"
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
	if cfg.Mocks.FailRate != 0 {
		t.Errorf("Mocks.FailRate = %v, want 0", cfg.Mocks.FailRate)
	}
	if !cfg.FeatureRequests {
		t.Error("FeatureRequests = false, want on by default")
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
		"MOCKS_ADDR":           "mocks:9091",
		"MOCKS_GRPC_ADDR":      ":19091",
		"MOCKS_HTTP_ADDR":      ":18090",
		"MOCKS_STATE_FILE":     "/data/state.json",
		"MOCK_MAX_LATENCY":     "200ms",
		"SESSION_SECRET":       "s3",
		"MOCK_FAIL_RATE":       "0.25",
		"HTTP_ADDR":            ":18080",
		"GRPC_ADDR":            ":19090",
		"FEATURE_REQUESTS":     "false",
		"PLAYBOOK_REPO":        "",
		"PLAYBOOK_REF":         "main",
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
			Endpoints: []Endpoint{{
				Name: "default", BaseURL: "http://ollama:11434/v1", Model: "qwen3.6:35b-a3b", APIKey: "secret",
				MaxConcurrency: 1, ReasoningEffort: "none",
			}},
			QueueTimeout: 5 * time.Minute,
		},
		TurnTimeout:       80 * time.Second,
		WorkerConcurrency: 4,
		Embeddings:        Embeddings{Enabled: true, Model: "bge-m3"},
		DatabaseURL:       "postgres://buddy:buddy@postgres:5432/buddy",
		RedisAddr:         "redis:6379",
		MocksAddr:         "mocks:9091",
		SessionSecret:     "s3",
		Mocks: Mocks{
			GRPCAddr: ":19091", HTTPAddr: ":18090", FailRate: 0.25, MaxLatency: 200 * time.Millisecond,
			StateFile: "/data/state.json",
		},
		HTTPAddr:        ":18080",
		GRPCAddr:        ":19090",
		FeatureRequests: false,
		PlaybookRef:     "main",
	}
	if !reflect.DeepEqual(cfg, want) {
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
		"bad latency":              {"MOCK_MAX_LATENCY": "slow"},
		"negative latency":         {"MOCK_MAX_LATENCY": "-1s"},
		"empty provider value":     {"LLM_PROVIDER": ""},
		"unknown reasoning effort": {"LLM_REASONING_EFFORT": "max"},
		"unknown detailed effort":  {"LLM_REASONING_EFFORT_DETAILED": "max"},
		"temperature not a number": {"LLM_TEMPERATURE": "hot"},
		"temperature too high":     {"LLM_TEMPERATURE": "2.5"},
		"bad feature flag":         {"FEATURE_REQUESTS": "maybe"},
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

func TestLoadReadsModelTuning(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{"LLM_TEMPERATURE": "0.3", "LLM_REASONING_EFFORT_DETAILED": "low"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.Temperature == nil || *cfg.LLM.Temperature != 0.3 || cfg.LLM.DetailedReasoningEffort != "low" {
		t.Fatalf("LLM = %+v", cfg.LLM)
	}
}
