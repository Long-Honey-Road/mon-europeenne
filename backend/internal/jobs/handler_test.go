package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/agent"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/people"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

var liza = domain.User{ID: "u-liza", Login: "intern", DisplayName: "Лиза", Role: domain.RoleIntern, TeamID: "t-messenger"}

type users map[string]domain.User

func (u users) User(id string) (domain.User, error) {
	if v, ok := u[id]; ok {
		return v, nil
	}
	return domain.User{}, fmt.Errorf("unknown user %s", id)
}

func directory() *people.Memory {
	return people.NewMemory(
		[]domain.Team{{ID: "t-messenger", Name: "Messenger", Cluster: "Коммуникации"}},
		[]domain.Person{
			{ID: "p-anna", Name: "Анна", TeamID: "t-messenger"},
			{ID: "p-igor", Name: "Игорь", TeamID: "t-messenger", AskMeAbout: []string{"деплой на staging"}},
		},
		map[string]people.Relations{"u-liza": {MentorID: "p-anna"}},
	)
}

// memStore is the slice of the store the handler uses.
type memStore struct {
	mu       sync.Mutex
	threads  map[string]domain.Thread
	messages map[string][]domain.Message
	events   []domain.Event
	traces   []domain.TurnTrace
	traceErr error
}

func (s *memStore) SaveTrace(_ context.Context, t domain.TurnTrace) error {
	s.traces = append(s.traces, t)
	return s.traceErr
}

func newStore(msgs ...string) (*memStore, string) {
	s := &memStore{threads: map[string]domain.Thread{}, messages: map[string][]domain.Message{}}
	s.threads["t1"] = domain.Thread{ID: "t1", UserID: "u-liza", Type: domain.ThreadGeneral}
	for i, content := range msgs {
		role := domain.MessageUser
		if i%2 == 1 {
			role = domain.MessageAssistant
		}
		s.messages["t1"] = append(s.messages["t1"], domain.Message{ID: fmt.Sprint("m", i), ThreadID: "t1", Role: role, Content: content})
	}
	return s, fmt.Sprint("m", len(msgs)-1)
}

func (s *memStore) Thread(_ context.Context, userID, id string) (domain.Thread, error) {
	t, ok := s.threads[id]
	if !ok || t.UserID != userID {
		return domain.Thread{}, store.ErrNotFound
	}
	return t, nil
}

func (s *memStore) Messages(_ context.Context, threadID string, limit int) ([]domain.Message, error) {
	all := s.messages[threadID]
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

func (s *memStore) AddMessage(_ context.Context, m domain.Message) (domain.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m.ID, m.CreatedAt = fmt.Sprint("saved", len(s.messages[m.ThreadID])), time.Now()
	s.messages[m.ThreadID] = append(s.messages[m.ThreadID], m)
	return m, nil
}

func (s *memStore) AppendEvents(_ context.Context, _ string, events []domain.Event) error {
	s.events = append(s.events, events...)
	return nil
}

type recorder struct {
	mu     sync.Mutex
	events []stream.Event
}

func (r *recorder) Publish(_ context.Context, _ string, e stream.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

func (r *recorder) types() []string {
	var out []string
	for _, e := range r.events {
		if n := len(out); n > 0 && out[n-1] == e.Type && e.Type == "delta" {
			continue
		}
		out = append(out, e.Type)
	}
	return out
}

func handler(t *testing.T, st *memStore, model llm.Client) (*Handler, *recorder) {
	t.Helper()
	who, err := tools.WhoToAsk(directory())
	if err != nil {
		t.Fatalf("who_to_ask: %v", err)
	}
	reg, err := tools.NewRegistry(who)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	rec := &recorder{}
	return NewHandler(Deps{
		Store: st, Publisher: rec, Agent: agent.New(model, reg, agent.Config{}),
		Users: users{"u-liza": liza}, Directory: directory(),
	}), rec
}

func task(t *testing.T, turn ChatTurn) *asynq.Task {
	t.Helper()
	raw, err := json.Marshal(turn)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return asynq.NewTask(TypeChatTurn, raw)
}

func TestTurnStreamsAndSavesTheAnswer(t *testing.T) {
	st, msgID := newStore("Привет")
	model := fake.New(fake.Text("Привет! Я buddy, спрашивай."))
	h, rec := handler(t, st, model)

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID, Mode: "brief"})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if got := rec.types(); !slices.Equal(got, []string{"delta", "done"}) {
		t.Errorf("published %v", got)
	}
	saved := st.messages["t1"][1]
	if saved.Role != domain.MessageAssistant || saved.Content != "Привет! Я buddy, спрашивай." || string(saved.Blocks) != "[]" {
		t.Errorf("saved = %+v (blocks %s)", saved, saved.Blocks)
	}
	done := rec.events[len(rec.events)-1].Data
	if done["message_id"] != saved.ID || done["text"] != saved.Content {
		t.Errorf("done = %v", done)
	}
	if len(st.events) == 0 || st.events[0].Type != domain.EventMessageSent {
		t.Errorf("events = %+v", st.events)
	}
	system := model.Calls()[0].Messages[0].Content
	if !strings.Contains(system, `"name":"Лиза"`) {
		t.Errorf("profile missing from the prompt: %s", system)
	}
}

func TestTurnPublishesAndSavesBlocks(t *testing.T) {
	st, msgID := newStore("Кто поможет с деплоем на staging?")
	model := fake.New(fake.ToolCall("who_to_ask", map[string]string{"topic": "деплой на staging"}), fake.Text("Спроси Игоря."))
	h, rec := handler(t, st, model)

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	if got := rec.types(); !slices.Equal(got, []string{"status", "delta", "block", "done"}) {
		t.Errorf("published %v", got)
	}
	var blocks []map[string]any
	if err := json.Unmarshal(st.messages["t1"][1].Blocks, &blocks); err != nil || len(blocks) != 1 || blocks[0]["person_id"] != "p-igor" {
		t.Errorf("saved blocks = %s", st.messages["t1"][1].Blocks)
	}

	if len(st.traces) != 1 {
		t.Fatalf("traces = %+v", st.traces)
	}
	tr := st.traces[0]
	if tr.QuestionID != msgID || tr.AnswerID != st.messages["t1"][1].ID || tr.FinalText != "Спроси Игоря." || tr.Mode != "brief" ||
		!strings.Contains(tr.SystemPrompt, "buddy") || tr.Error != "" {
		t.Errorf("trace = %+v", tr)
	}
	var steps []struct {
		Tools []struct {
			Name, Outcome, Result string
			Args                  map[string]string
		}
	}
	if err := json.Unmarshal(tr.Steps, &steps); err != nil || len(steps) != 2 || len(steps[0].Tools) != 1 ||
		steps[0].Tools[0].Args["topic"] != "деплой на staging" || !strings.Contains(steps[0].Tools[0].Result, "p-igor") {
		t.Errorf("steps = %s (%v)", tr.Steps, err)
	}
}

func TestTurnSendsOnlyHistoryBeforeTheMessage(t *testing.T) {
	st, _ := newStore("первый", "ответ", "второй", "лишний ответ")
	model := fake.New(fake.Text("ок"))
	h, _ := handler(t, st, model)

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: "m2"})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	var got []string
	for _, m := range model.Calls()[0].Messages[1:] {
		got = append(got, string(m.Role)+":"+m.Content)
	}
	if !slices.Equal(got, []string{"user:первый", "assistant:ответ", "user:второй"}) {
		t.Fatalf("model saw %v", got)
	}
}

func TestModelFailurePublishesErrorWithEscalation(t *testing.T) {
	st, msgID := newStore("Привет")
	h, rec := handler(t, st, fake.New(fake.Fail(llm.ErrUnavailable)))

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}

	last := rec.events[len(rec.events)-1]
	if last.Type != "error" || last.Data["code"] != "model_unavailable" {
		t.Fatalf("last event = %+v", last)
	}
	chain, _ := last.Data["escalation"].([]tools.EscalationStep)
	if len(chain) == 0 || chain[0].Name != "Анна" {
		t.Errorf("escalation = %v", last.Data["escalation"])
	}
	if len(st.messages["t1"]) != 1 {
		t.Errorf("answer saved after a failure: %+v", st.messages["t1"])
	}
	if len(st.traces) != 1 || !strings.Contains(st.traces[0].Error, "unavailable") || st.traces[0].AnswerID != "" {
		t.Errorf("failed turn trace = %+v", st.traces)
	}
}

func TestLostTraceDoesNotFailTheTurn(t *testing.T) {
	st, msgID := newStore("Привет")
	st.traceErr = errors.New("db down")
	h, rec := handler(t, st, fake.New(fake.Text("ок")))

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	if got := rec.types(); got[len(got)-1] != "done" {
		t.Fatalf("published %v", got)
	}
}

func TestTurnForMissingThreadIsSkipped(t *testing.T) {
	st, msgID := newStore("Привет")
	h, rec := handler(t, st, fake.New())

	for _, turn := range []ChatTurn{
		{ThreadID: "t-none", UserID: "u-liza", MessageID: msgID},
		{ThreadID: "t1", UserID: "u-other", MessageID: msgID},
	} {
		if err := h.ProcessTask(context.Background(), task(t, turn)); err != nil {
			t.Errorf("%+v: %v", turn, err)
		}
	}
	if len(rec.events) != 0 {
		t.Errorf("published %v", rec.types())
	}
}

func TestTurnForUnknownMessageFails(t *testing.T) {
	st, _ := newStore("Привет")
	h, rec := handler(t, st, fake.New())

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: "m-gone"})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	if got := rec.types(); !slices.Equal(got, []string{"error"}) || rec.events[0].Data["code"] != "turn_failed" {
		t.Fatalf("published %+v", rec.events)
	}
}

func TestBadPayloadIsAnError(t *testing.T) {
	st, _ := newStore("Привет")
	h, _ := handler(t, st, fake.New())
	if err := h.ProcessTask(context.Background(), asynq.NewTask(TypeChatTurn, []byte("{"))); err == nil {
		t.Fatal("bad payload accepted")
	}
}
