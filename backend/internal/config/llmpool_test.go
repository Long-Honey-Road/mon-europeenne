package config

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestLegacyModelIsAPoolOfOne(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"LLM_BASE_URL": "http://ollama:11434/v1", "LLM_MODEL": "qwen", "LLM_API_KEY": "k", "LLM_REASONING_EFFORT": "none",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []Endpoint{{Name: "default", BaseURL: "http://ollama:11434/v1", Model: "qwen", APIKey: "k", MaxConcurrency: 1, ReasoningEffort: "none"}}
	if !slices.Equal(cfg.LLM.Endpoints, want) {
		t.Errorf("Endpoints = %+v, want %+v", cfg.LLM.Endpoints, want)
	}
	if cfg.LLM.QueueTimeout != 5*time.Minute || cfg.TurnTimeout != 80*time.Second || cfg.WorkerConcurrency != 4 {
		t.Errorf("queue %v, turn %v, workers %d", cfg.LLM.QueueTimeout, cfg.TurnTimeout, cfg.WorkerConcurrency)
	}
}

func TestLoadReadsEndpoints(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"LLM_BASE_URL":         "http://ignored/v1",
		"LLM_REASONING_EFFORT": "none",
		"LLM_ENDPOINTS": `[
			{"name":"pc","base_url":"http://pc:11434/v1","model":"qwen3.6:35b-a3b","max_concurrency":1},
			{"name":"uni","base_url":"https://llm.uni/v1","api_key":"t","model":"qwen3","max_concurrency":4,"reasoning_effort":"low"}
		]`,
		"LLM_MAX_CONCURRENCY": "3",
		"LLM_QUEUE_TIMEOUT":   "2m",
		"TURN_TIMEOUT":        "100s",
		"WORKER_CONCURRENCY":  "8",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []Endpoint{
		{Name: "pc", BaseURL: "http://pc:11434/v1", Model: "qwen3.6:35b-a3b", MaxConcurrency: 1, ReasoningEffort: "none"},
		{Name: "uni", BaseURL: "https://llm.uni/v1", APIKey: "t", Model: "qwen3", MaxConcurrency: 4, ReasoningEffort: "low"},
	}
	if !slices.Equal(cfg.LLM.Endpoints, want) {
		t.Errorf("Endpoints = %+v, want %+v", cfg.LLM.Endpoints, want)
	}
	if cfg.LLM.QueueTimeout != 2*time.Minute || cfg.TurnTimeout != 100*time.Second || cfg.WorkerConcurrency != 8 {
		t.Errorf("queue %v, turn %v, workers %d", cfg.LLM.QueueTimeout, cfg.TurnTimeout, cfg.WorkerConcurrency)
	}
}

func TestEndpointDefaultsToOneSlot(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{"LLM_ENDPOINTS": `[{"base_url":"http://a/v1","model":"m"}]`}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.LLM.Endpoints[0]; got.MaxConcurrency != 1 || got.Name != "llm-1" {
		t.Errorf("endpoint = %+v", got)
	}
}

func TestLoadRejectsInvalidPool(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"endpoints not JSON":    {"LLM_ENDPOINTS": "http://a"},
		"endpoints empty":       {"LLM_ENDPOINTS": "[]"},
		"endpoint without url":  {"LLM_ENDPOINTS": `[{"model":"m"}]`},
		"endpoint no model":     {"LLM_ENDPOINTS": `[{"base_url":"http://a/v1"}]`},
		"endpoint bad slots":    {"LLM_ENDPOINTS": `[{"base_url":"http://a/v1","model":"m","max_concurrency":-1}]`},
		"endpoint bad effort":   {"LLM_ENDPOINTS": `[{"base_url":"http://a/v1","model":"m","reasoning_effort":"max"}]`},
		"endpoint unknown key":  {"LLM_ENDPOINTS": `[{"base_url":"http://a/v1","model":"m","maxConcurrency":2}]`},
		"legacy slots":          {"LLM_MAX_CONCURRENCY": "0"},
		"queue timeout":         {"LLM_QUEUE_TIMEOUT": "soon"},
		"turn timeout":          {"TURN_TIMEOUT": "0s"},
		"workers":               {"WORKER_CONCURRENCY": "-2"},
		"duplicate names":       {"LLM_ENDPOINTS": `[{"name":"a","base_url":"http://a/v1","model":"m"},{"name":"a","base_url":"http://b/v1","model":"m"}]`},
		"negative queue":        {"LLM_QUEUE_TIMEOUT": "-1m"},
		"workers not a number":  {"WORKER_CONCURRENCY": "four"},
		"legacy slots non-numb": {"LLM_MAX_CONCURRENCY": "x"},
	} {
		if _, err := Load(envOf(env)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}
