// Package agent runs one assistant turn: model, tools, citations and server-built blocks.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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
	// Today is the date line for the prompt, e.g. from TodayLine.
	Today   string
	Pinned  string
	Summary string
	// Sphere, SphereMemory and Overview are the sphere context of a tab (design §6.5), empty elsewhere.
	Sphere       string
	SphereMemory string
	Overview     string
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
	// Action is a form the user confirms, e.g. {kind: request, type_key, title, fields, approvers}.
	Action json.RawMessage `json:"action,omitempty"`
}

var weekdays = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}

// companyZone is where the day changes for the company: Moscow time.
var companyZone = time.FixedZone("MSK", 3*60*60)

// TodayLine tells the model the date and the day of onboarding, counted from the start date (1-based).
func TodayLine(now, start time.Time) string {
	now = now.In(companyZone)
	line := fmt.Sprintf("%s, %s", now.Format(time.DateOnly), weekdays[now.Weekday()])
	if start.IsZero() {
		return line
	}
	y, m, d := start.Date()
	ny, nm, nd := now.Date()
	days := int(time.Date(ny, nm, nd, 0, 0, 0, 0, time.UTC).Sub(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)).Hours() / 24)
	if days < 0 {
		return fmt.Sprintf("%s; стажировка начнётся %s", line, start.Format(time.DateOnly))
	}
	return fmt.Sprintf("%s; день стажировки %d", line, days+1)
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
	// UIEvents are screen updates the tools asked for, published to the thread after the answer is saved.
	UIEvents []tools.UIEvent
	// Trace is filled even when Run fails, as far as the turn got.
	Trace Trace
}

// maxTracedResult keeps traces readable: tool results are cut to this many bytes.
const maxTracedResult = 4000

// Trace is how the turn went, for debugging: every model call with its tools, and the text before the invariants.
type Trace struct {
	SystemPrompt string        `json:"system_prompt"`
	Steps        []TraceStep   `json:"steps"`
	RawText      string        `json:"raw_text"`
	Violations   []string      `json:"violations"`
	Duration     time.Duration `json:"duration"`
}

type TraceStep struct {
	DurationMS       int64  `json:"duration_ms"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	ReasoningEffort  string `json:"reasoning_effort,omitempty"`
	// Text is what the model wrote in this call, including text it withdrew to call tools.
	Text  string      `json:"text,omitempty"`
	Tools []TraceTool `json:"tools,omitempty"`
}

type TraceTool struct {
	Name       string          `json:"name"`
	Args       json.RawMessage `json:"args"`
	Outcome    string          `json:"outcome"`
	Result     string          `json:"result"`
	DurationMS int64           `json:"duration_ms"`
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	for !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…"
}

type Config struct {
	// MaxToolSteps model calls may request tools; the next call goes without tools.
	MaxToolSteps int
	Timeout      time.Duration
	HistoryLimit int
	// DetailedEffort is the reasoning effort for detailed answers; empty keeps the client default.
	DetailedEffort string
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
		cfg.Timeout = 80 * time.Second
	}
	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = 12
	}
	return &Agent{model: model, tools: t, cfg: cfg}
}

// Run executes one turn: tool calls until the model answers with text, then server-side blocks.
func (a *Agent) Run(ctx context.Context, t Turn, emit Emitter) (Result, error) {
	return a.RunWith(ctx, a.model, t, emit)
}

// RunWith is Run on a model held for this turn, e.g. a pool endpoint; the timeout starts here.
func (a *Agent) RunWith(ctx context.Context, model llm.Client, t Turn, emit Emitter) (Result, error) {
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
	started := time.Now()
	trace := Trace{SystemPrompt: system, Steps: []TraceStep{}, Violations: []string{}}
	failed := func(err error) (Result, error) {
		trace.Duration = time.Since(started)
		return Result{Trace: trace}, err
	}

	st := &turnState{}
	st.event(domain.EventMessageSent, map[string]any{"thread": t.Thread.String()})

	msgs := []llm.Message{{Role: llm.RoleSystem, Content: system}}
	msgs = append(msgs, tail(t.History, a.cfg.HistoryLimit)...)
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: t.Message})
	offered := a.tools.Available(t.Principal, t.Thread)

	var text string
	for step := 0; ; step++ {
		req := llm.Request{Messages: msgs}
		if t.Mode == ModeDetailed {
			req.ReasoningEffort = a.cfg.DetailedEffort
		}
		if step < a.cfg.MaxToolSteps {
			req.Tools = offered
		}
		streamed := false
		callStart := time.Now()
		resp, err := model.ChatStream(ctx, req, func(d string) {
			streamed = true
			emit.Delta(d)
		})
		ts := TraceStep{DurationMS: time.Since(callStart).Milliseconds(), ReasoningEffort: req.ReasoningEffort}
		if err != nil {
			trace.Steps = append(trace.Steps, ts)
			return failed(fmt.Errorf("model: %w", err))
		}
		ts.PromptTokens, ts.CompletionTokens, ts.Text = resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Message.Content
		if len(resp.Message.ToolCalls) == 0 || len(req.Tools) == 0 {
			trace.Steps = append(trace.Steps, ts)
			text = resp.Message.Content
			break
		}
		if streamed {
			emit.Reset()
		}
		msgs = append(msgs, resp.Message)
		for _, call := range resp.Message.ToolCalls {
			emit.Status(statusText(call.Name))
			toolStart := time.Now()
			content, outcome := a.callTool(ctx, t, call, st)
			ts.Tools = append(ts.Tools, TraceTool{Name: call.Name, Args: call.Arguments, Outcome: outcome,
				Result: clip(content, maxTracedResult), DurationMS: time.Since(toolStart).Milliseconds()})
			st.event(domain.EventToolCalled, map[string]any{"tool": call.Name, "outcome": outcome})
			msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Content: content})
		}
		trace.Steps = append(trace.Steps, ts)
	}
	trace.RawText = text

	text, cited := st.finishText(text, t.Thread)
	blocks := st.blocks(cited)
	for _, b := range blocks {
		emit.Block(b)
	}
	st.closingEvents(blocks)
	trace.Violations = append(trace.Violations, st.violations...)
	trace.Duration = time.Since(started)
	return Result{Text: text, Mode: t.Mode, Blocks: blocks, Events: st.events, UIEvents: st.uiEvents, Trace: trace}, nil
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
	"pin_note":           "закрепляю",
	"highlight_spheres":  "подсвечиваю на карте",
	"focus_sphere":       "предлагаю пункт плана",
	"read_repo":          "читаю репозиторий",
	"get_team_process":   "смотрю процессы команды",
	"who_to_ask":         "ищу, к кому обратиться",
	"team_members":       "смотрю состав команды",
	"report_not_found":   "собираю, к кому обратиться",
	"general_knowledge":  "вспоминаю общие знания",
	"get_plan":           "смотрю план адаптации",
	"update_item_status": "обновляю план",
	"get_request_types":  "смотрю виды заявок",
	"prepare_request":    "готовлю заявку",
	"get_request_status": "проверяю заявку",
	"list_my_requests":   "смотрю твои заявки",
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
