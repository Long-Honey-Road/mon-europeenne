package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/openaicompat"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/pool"
)

var errProbesFailed = errors.New("some probes failed")

// newLLM builds the model pool: one client per configured endpoint.
func newLLM(cfg config.LLM) (*pool.Pool, error) {
	endpoints := cfg.Endpoints
	if len(endpoints) == 0 {
		endpoints = []config.Endpoint{{
			Name: "default", BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey, ReasoningEffort: cfg.ReasoningEffort,
		}}
	}
	eps := make([]pool.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		var client llm.Client
		switch cfg.Provider {
		case config.ProviderOpenAICompat:
			client = openaicompat.New(openaicompat.Config{
				BaseURL:         ep.BaseURL,
				Model:           ep.Model,
				APIKey:          ep.APIKey,
				ReasoningEffort: ep.ReasoningEffort,
				Temperature:     cfg.Temperature,
				MaxRetries:      1,
			})
		case config.ProviderFake:
			client = fake.NewEcho(fakeAnswer)
		default:
			return nil, fmt.Errorf("provider %q: %w", cfg.Provider, errNotImplemented)
		}
		eps = append(eps, pool.Endpoint{Name: ep.Name, Client: client, MaxConcurrency: ep.MaxConcurrency})
	}
	return pool.New(eps, pool.Config{})
}

type probe struct {
	name   string
	req    llm.Request
	stream bool
	check  func(msg llm.Message, deltas int) error
}

var probes = []probe{
	{
		name: "text",
		req:  llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: "Ответь одним словом: столица Франции?"}}},
		check: func(m llm.Message, _ int) error {
			if m.Content == "" {
				return errors.New("empty answer")
			}
			return nil
		},
	},
	{
		name: "tool call",
		req: llm.Request{
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "Отвечай только по документам команды. Сначала ищи."},
				{Role: llm.RoleUser, Content: "Как поднять dev-окружение в нашей команде?"},
			},
			Tools: []llm.Tool{{
				Name:        "search_docs",
				Description: "Search team documents",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
			}},
		},
		check: func(m llm.Message, _ int) error {
			if len(m.ToolCalls) == 0 || m.ToolCalls[0].Name != "search_docs" {
				return fmt.Errorf("expected a search_docs call, got %q", m.Content)
			}
			return nil
		},
	},
	{
		name: "json",
		req: llm.Request{
			Messages: []llm.Message{{Role: llm.RoleUser, Content: "Столица Франции?"}},
			Format: &llm.Format{
				Name:   "reply",
				Schema: json.RawMessage(`{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`),
			},
		},
		check: func(m llm.Message, _ int) error {
			var out struct {
				Answer string `json:"answer"`
			}
			if err := json.Unmarshal([]byte(m.Content), &out); err != nil || out.Answer == "" {
				return fmt.Errorf("not the requested JSON: %q", m.Content)
			}
			return nil
		},
	},
	{
		name:   "stream",
		stream: true,
		req:    llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: "Перечисли через запятую три столицы Европы."}}},
		check: func(m llm.Message, deltas int) error {
			if m.Content == "" || deltas < 2 {
				return fmt.Errorf("want text in several pieces, got %d pieces", deltas)
			}
			return nil
		},
	},
}

// runLLMCheck sends one probe per capability the agent relies on to the configured model.
func runLLMCheck(_ []string, stdout io.Writer) error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}
	models, err := newLLM(cfg.LLM)
	if err != nil {
		return err
	}

	failed := false
	for i, ep := range models.Endpoints() {
		if i < len(cfg.LLM.Endpoints) {
			c := cfg.LLM.Endpoints[i]
			fmt.Fprintf(stdout, "endpoint %s: model %s at %s, reasoning effort %q, %d at once\n", ep.Name, c.Model, c.BaseURL, c.ReasoningEffort, ep.MaxConcurrency)
		}
		if !probeModel(ep.Client, stdout) {
			failed = true
		}
	}
	if failed {
		return errProbesFailed
	}
	return nil
}

// probeModel runs every probe against one endpoint and reports whether all passed.
func probeModel(client llm.Client, stdout io.Writer) bool {
	passed := true
	for _, p := range probes {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		start := time.Now()
		deltas := 0
		var resp llm.Response
		var err error
		if p.stream {
			resp, err = client.ChatStream(ctx, p.req, func(string) { deltas++ })
		} else {
			resp, err = client.Chat(ctx, p.req)
		}
		cancel()
		if err == nil {
			err = p.check(resp.Message, deltas)
		}
		if err != nil {
			passed = false
			fmt.Fprintf(stdout, "%s: FAIL %v\n", p.name, err)
			continue
		}
		fmt.Fprintf(stdout, "%s: ok %s, %d tokens\n", p.name, time.Since(start).Round(time.Millisecond), resp.Usage.CompletionTokens)
	}
	return passed
}
