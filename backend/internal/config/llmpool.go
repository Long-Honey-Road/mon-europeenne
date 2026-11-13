package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Endpoint is one model source of the LLM pool; LLM_ENDPOINTS is a JSON array of them.
type Endpoint struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	// MaxConcurrency is how many turns the endpoint serves at once; 1 if unset.
	MaxConcurrency int `json:"max_concurrency"`
	// ReasoningEffort overrides LLM_REASONING_EFFORT for this endpoint.
	ReasoningEffort string `json:"reasoning_effort"`
}

// loadPool reads the endpoints, queue and turn limits; without LLM_ENDPOINTS the LLM_BASE_URL model is a pool of one.
func loadPool(cfg *Config, str func(key, def string) string) error {
	var err error
	if cfg.LLM.QueueTimeout, err = positiveDuration("LLM_QUEUE_TIMEOUT", str("LLM_QUEUE_TIMEOUT", "5m")); err != nil {
		return err
	}
	if cfg.TurnTimeout, err = positiveDuration("TURN_TIMEOUT", str("TURN_TIMEOUT", "80s")); err != nil {
		return err
	}
	if cfg.WorkerConcurrency, err = positiveInt("WORKER_CONCURRENCY", str("WORKER_CONCURRENCY", "4")); err != nil {
		return err
	}

	raw := str("LLM_ENDPOINTS", "")
	if raw == "" {
		slots, err := positiveInt("LLM_MAX_CONCURRENCY", str("LLM_MAX_CONCURRENCY", "1"))
		if err != nil {
			return err
		}
		cfg.LLM.Endpoints = []Endpoint{{
			Name: "default", BaseURL: cfg.LLM.BaseURL, Model: cfg.LLM.Model, APIKey: cfg.LLM.APIKey,
			MaxConcurrency: slots, ReasoningEffort: cfg.LLM.ReasoningEffort,
		}}
		return nil
	}

	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	var eps []Endpoint
	if err := dec.Decode(&eps); err != nil {
		return fmt.Errorf("LLM_ENDPOINTS: %w: %w", ErrInvalid, err)
	}
	if len(eps) == 0 {
		return fmt.Errorf("LLM_ENDPOINTS is empty: %w", ErrInvalid)
	}
	names := map[string]bool{}
	for i := range eps {
		ep := &eps[i]
		if ep.Name == "" {
			ep.Name = fmt.Sprintf("llm-%d", i+1)
		}
		if ep.MaxConcurrency == 0 {
			ep.MaxConcurrency = 1
		}
		if ep.ReasoningEffort == "" {
			ep.ReasoningEffort = cfg.LLM.ReasoningEffort
		}
		switch {
		case ep.BaseURL == "" || ep.Model == "":
			return fmt.Errorf("LLM_ENDPOINTS[%d] needs base_url and model: %w", i, ErrInvalid)
		case ep.MaxConcurrency < 0:
			return fmt.Errorf("LLM_ENDPOINTS[%d].max_concurrency=%d: %w", i, ep.MaxConcurrency, ErrInvalid)
		case !validEffort(ep.ReasoningEffort):
			return fmt.Errorf("LLM_ENDPOINTS[%d].reasoning_effort=%q: %w", i, ep.ReasoningEffort, ErrInvalid)
		case names[ep.Name]:
			return fmt.Errorf("LLM_ENDPOINTS name %q repeats: %w", ep.Name, ErrInvalid)
		}
		names[ep.Name] = true
	}
	cfg.LLM.Endpoints = eps
	return nil
}

func validEffort(v string) bool {
	switch v {
	case "", "none", "minimal", "low", "medium", "high":
		return true
	}
	return false
}

func positiveDuration(key, v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s=%q: %w", key, v, ErrInvalid)
	}
	return d, nil
}

func positiveInt(key, v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s=%q: %w", key, v, ErrInvalid)
	}
	return n, nil
}
