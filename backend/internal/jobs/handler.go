package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hibiken/asynq"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/agent"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/profile"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/spheres"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

// historyWindow messages are read so the agent's own limit (12) still has a full tail before the question.
const historyWindow = 40

const (
	queuedStatus        = "В очереди…"
	defaultQueueTimeout = 5 * time.Minute
)

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
	RunWith(ctx context.Context, model llm.Client, t agent.Turn, emit agent.Emitter) (agent.Result, error)
}

// Models hands out one model endpoint for a whole turn; onWait is called if the turn has to queue.
type Models interface {
	Acquire(ctx context.Context, onWait func()) (llm.Client, func(), error)
}

// Turns frees the thread for the next message once the turn ends.
type Turns interface {
	Release(ctx context.Context, threadID, jobID string) error
}

type Users interface {
	User(ctx context.Context, id string) (domain.User, error)
}

// Spheres builds the context of a tab with spheres (design §6.5) and schedules summaries after a turn.
type Spheres interface {
	For(ctx context.Context, th domain.Thread, msg domain.Message) (spheres.Context, error)
	AfterTurn(ctx context.Context, th domain.Thread, f domain.Focus)
}

type Deps struct {
	Store     Store
	Publisher Publisher
	Agent     Runner
	Users     Users
	Directory tools.PeopleDirectory
	Models    Models
	Turns     Turns
	// Spheres is nil where tabs have no spheres: the turn then sees the plain thread tail.
	Spheres Spheres
	// StandRules returns a test stand's extra prompt rules for each turn; nil or "" means none.
	StandRules func() string
	// QueueTimeout limits waiting for a free model; 5 min if zero.
	QueueTimeout time.Duration
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
	var once sync.Once
	free := func() {
		once.Do(func() {
			if err := h.d.Turns.Release(context.WithoutCancel(ctx), turn.ThreadID, jobID); err != nil {
				slog.Error("free thread after turn", "thread", turn.ThreadID, "err", err)
			}
		})
	}
	defer free()
	emit := &busEmitter{ctx: ctx, pub: h.d.Publisher, threadID: turn.ThreadID, jobID: jobID, beforeEnd: free}

	thread, err := h.d.Store.Thread(ctx, turn.UserID, turn.ThreadID)
	if errors.Is(err, store.ErrNotFound) {
		slog.Warn("chat_turn for a missing thread", "thread", turn.ThreadID, "user", turn.UserID)
		return nil
	}
	if err != nil {
		return err
	}
	user, err := h.d.Users.User(ctx, turn.UserID)
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

	model, release, err := h.acquire(ctx, emit)
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		h.saveTrace(ctx, domain.TurnTrace{ThreadID: thread.ID, QuestionID: turn.MessageID, JobID: jobID, Mode: turn.Mode,
			Error: "queue: no free model: " + err.Error(), Steps: []byte("[]")})
		h.fail(emit, p, "model_busy", "Модель занята другими ответами и не освободилась. Попробуй ещё раз через пару минут или спроси у людей ниже.")
		return nil
	}
	at := agent.Turn{
		Principal: p, Thread: thread.Type, Mode: agent.Mode(turn.Mode), Profile: prof, Today: agent.TodayLine(time.Now(), user.StartDate),
		Pinned: pinText(thread.Pin), Summary: thread.Summary, History: history(msgs[:idx]), Message: msgs[idx].Content,
	}
	if h.d.StandRules != nil {
		at.StandRules = h.d.StandRules()
	}
	if err := h.withSphere(ctx, &at, thread, msgs[idx]); err != nil {
		release()
		return err
	}
	res, err := h.d.Agent.RunWith(ctx, model, at, emit)
	release()
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
		Focus: msgs[idx].Focus,
	})
	if err != nil {
		return err
	}
	if err := h.d.Store.AppendEvents(ctx, user.ID, domainEvents(res.Events)); err != nil {
		slog.Error("append events", "thread", thread.ID, "err", err)
	}
	trace.AnswerID = saved.ID
	h.saveTrace(ctx, trace)
	if h.d.Spheres != nil {
		h.d.Spheres.AfterTurn(ctx, thread, msgs[idx].Focus)
	}
	for _, e := range res.UIEvents {
		emit.publish(e.Type, e.Data)
	}
	// The saved text may differ from the stream: invariants remove invented links and markers.
	emit.publish("done", map[string]any{"message_id": saved.ID, "text": saved.Content})
	return nil
}

// sphereTabs are the thread types shown as a tab with spheres; other threads keep the plain tail and summary.
var sphereTabs = map[domain.ThreadType]bool{domain.ThreadPlan: true, domain.ThreadVault: true}

// withSphere swaps the thread tail for the sphere's own context: card, memory, overview and its messages.
func (h *Handler) withSphere(ctx context.Context, t *agent.Turn, th domain.Thread, msg domain.Message) error {
	if h.d.Spheres == nil || !sphereTabs[th.Type] {
		return nil
	}
	sc, err := h.d.Spheres.For(ctx, th, msg)
	if err != nil {
		return err
	}
	t.Overview = sc.Overview
	if !msg.Focus.IsZero() {
		t.Sphere, t.SphereMemory, t.Summary, t.History = sc.Card, sc.Memory, "", history(sc.History)
	} else {
		t.Summary = ""
	}
	return nil
}

// acquire waits for a model endpoint, telling the user the turn is queued; the turn timeout starts after it.
func (h *Handler) acquire(ctx context.Context, emit *busEmitter) (llm.Client, func(), error) {
	timeout := h.d.QueueTimeout
	if timeout == 0 {
		timeout = defaultQueueTimeout
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	model, release, err := h.d.Models.Acquire(waitCtx, func() { emit.Status(queuedStatus) })
	if wait := time.Since(started); wait > time.Second {
		slog.Info("turn waited for a model", "thread", emit.threadID, "wait", wait.Round(time.Millisecond), "err", err)
	}
	return model, release, err
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
	// beforeEnd frees the thread before done or error, so the client can send the next message at once.
	beforeEnd func()
}

func (e *busEmitter) publish(typ string, data map[string]any) {
	if (typ == "done" || typ == "error") && e.beforeEnd != nil {
		e.beforeEnd()
	}
	data["job_id"] = e.jobID
	if err := e.pub.Publish(e.ctx, e.threadID, stream.Event{Type: typ, Data: data}); err != nil {
		slog.Error("publish stream event", "type", typ, "thread", e.threadID, "err", err)
	}
}

func (e *busEmitter) Status(text string)  { e.publish("status", map[string]any{"text": text}) }
func (e *busEmitter) Delta(text string)   { e.publish("delta", map[string]any{"text": text}) }
func (e *busEmitter) Reset()              { e.publish("reset", map[string]any{}) }
func (e *busEmitter) Block(b agent.Block) { e.publish("block", map[string]any{"block": b}) }
