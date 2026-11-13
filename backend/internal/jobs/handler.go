package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/agent"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/profile"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

// historyWindow messages are read so the agent's own limit (12) still has a full tail before the question.
const historyWindow = 40

type Store interface {
	Thread(ctx context.Context, userID, id string) (domain.Thread, error)
	Messages(ctx context.Context, threadID string, limit int) ([]domain.Message, error)
	AddMessage(ctx context.Context, m domain.Message) (domain.Message, error)
	AppendEvents(ctx context.Context, actorID string, events []domain.Event) error
	SaveTrace(ctx context.Context, t domain.TurnTrace) error
}

type Publisher interface {
	Publish(ctx context.Context, threadID string, e stream.Event) error
}

type Runner interface {
	Run(ctx context.Context, t agent.Turn, emit agent.Emitter) (agent.Result, error)
}

type Users interface {
	User(id string) (domain.User, error)
}

type Deps struct {
	Store     Store
	Publisher Publisher
	Agent     Runner
	Users     Users
	Directory tools.PeopleDirectory
}

// Handler answers one user message: runs the agent, streams events, saves the answer.
type Handler struct{ d Deps }

func NewHandler(d Deps) *Handler { return &Handler{d: d} }

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var turn ChatTurn
	if err := json.Unmarshal(t.Payload(), &turn); err != nil {
		return fmt.Errorf("chat_turn payload: %w", err)
	}
	jobID, _ := asynq.GetTaskID(ctx)
	emit := &busEmitter{ctx: ctx, pub: h.d.Publisher, threadID: turn.ThreadID, jobID: jobID}

	thread, err := h.d.Store.Thread(ctx, turn.UserID, turn.ThreadID)
	if errors.Is(err, store.ErrNotFound) {
		slog.Warn("chat_turn for a missing thread", "thread", turn.ThreadID, "user", turn.UserID)
		return nil
	}
	if err != nil {
		return err
	}
	user, err := h.d.Users.User(turn.UserID)
	if err != nil {
		return err
	}
	p := domain.Principal{UserID: user.ID, Role: user.Role, TeamID: user.TeamID}

	msgs, err := h.d.Store.Messages(ctx, thread.ID, historyWindow)
	if err != nil {
		return err
	}
	idx := -1
	for i, m := range msgs {
		if m.ID == turn.MessageID {
			idx = i
		}
	}
	if idx < 0 {
		h.fail(emit, p, "turn_failed", "Сообщение не найдено, отправь его ещё раз.")
		return nil
	}
	prof, err := profile.Build(ctx, user, h.d.Directory)
	if err != nil {
		return err
	}

	res, err := h.d.Agent.Run(ctx, agent.Turn{
		Principal: p, Thread: thread.Type, Mode: agent.Mode(turn.Mode), Profile: prof, Today: agent.TodayLine(time.Now(), user.StartDate),
		Pinned: pinText(thread.Pin), Summary: thread.Summary, History: history(msgs[:idx]), Message: msgs[idx].Content,
	}, emit)
	trace := domain.TurnTrace{
		ThreadID: thread.ID, QuestionID: turn.MessageID, JobID: jobID, Mode: string(res.Mode), Duration: res.Trace.Duration,
		SystemPrompt: res.Trace.SystemPrompt, RawText: res.Trace.RawText, FinalText: res.Text, Violations: res.Trace.Violations,
	}
	if trace.Mode == "" {
		trace.Mode = turn.Mode
	}
	if steps, merr := json.Marshal(res.Trace.Steps); merr == nil {
		trace.Steps = steps
	}
	if err != nil {
		trace.Error = err.Error()
		h.saveTrace(ctx, trace)
		slog.Error("agent turn failed", "thread", thread.ID, "err", err)
		if errors.Is(err, llm.ErrUnavailable) || errors.Is(err, context.DeadlineExceeded) {
			h.fail(emit, p, "model_unavailable", "Модель сейчас не отвечает. Попробуй ещё раз или спроси у людей ниже.")
		} else {
			h.fail(emit, p, "turn_failed", "Не получилось ответить. Попробуй ещё раз или спроси у людей ниже.")
		}
		return nil
	}

	blocks, err := json.Marshal(orEmpty(res.Blocks))
	if err != nil {
		return err
	}
	saved, err := h.d.Store.AddMessage(ctx, domain.Message{
		ThreadID: thread.ID, Role: domain.MessageAssistant, Content: res.Text, Blocks: blocks, JobID: jobID,
	})
	if err != nil {
		return err
	}
	if err := h.d.Store.AppendEvents(ctx, user.ID, domainEvents(res.Events)); err != nil {
		slog.Error("append events", "thread", thread.ID, "err", err)
	}
	trace.AnswerID = saved.ID
	h.saveTrace(ctx, trace)
	// The saved text may differ from the stream: invariants remove invented links and markers.
	emit.publish("done", map[string]any{"message_id": saved.ID, "text": saved.Content})
	return nil
}

// saveTrace keeps the turn for debugging; losing a trace never fails the turn.
func (h *Handler) saveTrace(ctx context.Context, t domain.TurnTrace) {
	if err := h.d.Store.SaveTrace(ctx, t); err != nil {
		slog.Error("save turn trace", "thread", t.ThreadID, "err", err)
	}
}

// fail tells the client the turn failed and whom to ask instead.
func (h *Handler) fail(emit *busEmitter, p domain.Principal, code, message string) {
	chain, err := tools.EscalationChain(emit.ctx, h.d.Directory, p)
	if err != nil {
		slog.Error("escalation chain", "err", err)
	}
	emit.publish("error", map[string]any{"code": code, "message": message, "escalation": chain})
}

func history(msgs []domain.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		role := llm.RoleUser
		if m.Role == domain.MessageAssistant {
			role = llm.RoleAssistant
		}
		out = append(out, llm.Message{Role: role, Content: m.Content})
	}
	return out
}

func pinText(pin domain.ThreadPin) string {
	if pin == (domain.ThreadPin{}) {
		return ""
	}
	raw, _ := json.Marshal(pin)
	return string(raw)
}

func domainEvents(events []agent.Event) []domain.Event {
	out := make([]domain.Event, 0, len(events))
	for _, e := range events {
		payload, _ := json.Marshal(e.Payload)
		out = append(out, domain.Event{Type: e.Type, Payload: payload})
	}
	return out
}

func orEmpty(b []agent.Block) []agent.Block {
	if b == nil {
		return []agent.Block{}
	}
	return b
}

// busEmitter turns agent callbacks into stream events; publish failures are logged, the turn goes on.
type busEmitter struct {
	ctx      context.Context
	pub      Publisher
	threadID string
	jobID    string
}

func (e *busEmitter) publish(typ string, data map[string]any) {
	data["job_id"] = e.jobID
	if err := e.pub.Publish(e.ctx, e.threadID, stream.Event{Type: typ, Data: data}); err != nil {
		slog.Error("publish stream event", "type", typ, "thread", e.threadID, "err", err)
	}
}

func (e *busEmitter) Status(text string)  { e.publish("status", map[string]any{"text": text}) }
func (e *busEmitter) Delta(text string)   { e.publish("delta", map[string]any{"text": text}) }
func (e *busEmitter) Reset()              { e.publish("reset", map[string]any{}) }
func (e *busEmitter) Block(b agent.Block) { e.publish("block", map[string]any{"block": b}) }
