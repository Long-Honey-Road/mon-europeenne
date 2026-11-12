// Package agent runs one assistant turn: model, tools, citations and server-built blocks.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

var ErrEmptyMessage = errors.New("empty message")

type Mode string

const (
	ModeBrief    Mode = "brief"
	ModeDetailed Mode = "detailed"
)

// Emitter receives what the user sees while the turn runs.
type Emitter interface {
	Status(text string)
	Delta(text string)
	// Reset withdraws streamed text: the model wrote it and then went to a tool.
	Reset()
	Block(b Block)
}

// Tools is the registry as the agent sees it.
type Tools interface {
	Available(p domain.Principal, thread domain.ThreadType) []llm.Tool
	Call(ctx context.Context, p domain.Principal, thread domain.ThreadType, name string, args json.RawMessage) (json.RawMessage, error)
}

type Turn struct {
	Principal domain.Principal
	Thread    domain.ThreadType
	Mode      Mode
	// Profile is the caller's compact JSON profile.
	Profile string
	Pinned  string
	Summary string
	// History is the thread so far, oldest first; the agent keeps the tail.
	History []llm.Message
	Message string
}

type Source struct {
	Ref      int    `json:"ref"`
	SourceID string `json:"source_id"`
	Title    string `json:"title,omitempty"`
	URL      string `json:"url,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

// Block is a structured part of the answer, built by the server from tool results.
type Block struct {
	Type       string                 `json:"type"`
	Items      []Source               `json:"items,omitempty"`
	PersonID   string                 `json:"person_id,omitempty"`
	Name       string                 `json:"name,omitempty"`
	RoleTitle  string                 `json:"role_title,omitempty"`
	Reason     string                 `json:"reason,omitempty"`
	Contact    string                 `json:"contact,omitempty"`
	OtherTeam  bool                   `json:"other_team,omitempty"`
	Topic      string                 `json:"topic,omitempty"`
	Escalation []tools.EscalationStep `json:"escalation,omitempty"`
}

type Event struct {
	Type    domain.EventType
	Payload map[string]any
}

type Result struct {
	Text   string
	Mode   Mode
	Blocks []Block
	Events []Event
}

type Config struct {
	// MaxToolSteps model calls may request tools; the next call goes without tools.
	MaxToolSteps int
	Timeout      time.Duration
	HistoryLimit int
}

type Agent struct {
	model llm.Client
	tools Tools
	cfg   Config
}

func New(model llm.Client, t Tools, cfg Config) *Agent {
	if cfg.MaxToolSteps == 0 {
		cfg.MaxToolSteps = 6
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = 12
	}
	return &Agent{model: model, tools: t, cfg: cfg}
}

// Run executes one turn: tool calls until the model answers with text, then server-side blocks.
func (a *Agent) Run(ctx context.Context, t Turn, emit Emitter) (Result, error) {
	if strings.TrimSpace(t.Message) == "" {
		return Result{}, ErrEmptyMessage
	}
	if t.Mode == "" {
		t.Mode = ModeBrief
	}
	system, err := systemPrompt(t)
	if err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, a.cfg.Timeout)
	defer cancel()

	st := &turnState{}
	st.event(domain.EventMessageSent, map[string]any{"thread": t.Thread.String()})

	msgs := []llm.Message{{Role: llm.RoleSystem, Content: system}}
	msgs = append(msgs, tail(t.History, a.cfg.HistoryLimit)...)
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: t.Message})
	offered := a.tools.Available(t.Principal, t.Thread)

	var text string
	for step := 0; ; step++ {
		req := llm.Request{Messages: msgs}
		if step < a.cfg.MaxToolSteps {
			req.Tools = offered
		}
		streamed := false
		resp, err := a.model.ChatStream(ctx, req, func(d string) {
			streamed = true
			emit.Delta(d)
		})
		if err != nil {
			return Result{}, fmt.Errorf("model: %w", err)
		}
		if len(resp.Message.ToolCalls) == 0 || len(req.Tools) == 0 {
			text = resp.Message.Content
			break
		}
		if streamed {
			emit.Reset()
		}
		msgs = append(msgs, resp.Message)
		for _, call := range resp.Message.ToolCalls {
			emit.Status(statusText(call.Name))
			content, outcome := a.callTool(ctx, t, call, st)
			st.event(domain.EventToolCalled, map[string]any{"tool": call.Name, "outcome": outcome})
			msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Content: content})
		}
	}

	text, cited := st.finishText(text, t.Thread)
	blocks := st.blocks(cited)
	for _, b := range blocks {
		emit.Block(b)
	}
	st.closingEvents(blocks)
	return Result{Text: text, Mode: t.Mode, Blocks: blocks, Events: st.events}, nil
}

// callTool runs one call and returns what the model sees plus the outcome code for events.
func (a *Agent) callTool(ctx context.Context, t Turn, call llm.ToolCall, st *turnState) (string, string) {
	raw, err := a.tools.Call(ctx, t.Principal, t.Thread, call.Name, call.Arguments)
	if err != nil {
		code, msg := tools.Explain(err, call.Name, t.Thread)
		out, _ := json.Marshal(map[string]string{"error": code, "message": msg})
		return string(out), code
	}
	return string(st.absorb(call.Name, raw)), "ok"
}

var statusTexts = map[string]string{
	"search_docs":        "ищу в документах",
	"read_repo":          "читаю репозиторий",
	"get_team_process":   "смотрю процессы команды",
	"who_to_ask":         "ищу, к кому обратиться",
	"report_not_found":   "собираю, к кому обратиться",
	"get_plan":           "смотрю план адаптации",
	"update_item_status": "обновляю план",
	"get_request_types":  "смотрю виды заявок",
	"prepare_request":    "готовлю заявку",
	"get_request_status": "проверяю заявку",
}

func statusText(tool string) string {
	if s, ok := statusTexts[tool]; ok {
		return s
	}
	return "работаю: " + tool
}

func tail(history []llm.Message, n int) []llm.Message {
	if len(history) <= n {
		return history
	}
	return history[len(history)-n:]
}
