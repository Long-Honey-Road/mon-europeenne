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
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/pool"
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
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.threads[id]
	if !ok || t.UserID != userID {
		return domain.Thread{}, store.ErrNotFound
	}
	return t, nil
}

func (s *memStore) Messages(_ context.Context, threadID string, limit int) ([]domain.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
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

// memTurns records when the thread was freed, counted in published events.
type memTurns struct {
	rec      *recorder
	mu       sync.Mutex
	released map[string]int
}

func (m *memTurns) Release(_ context.Context, threadID, jobID string) error {
	m.rec.mu.Lock()
	n := len(m.rec.events)
	m.rec.mu.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.released[threadID+"/"+jobID]; !ok {
		m.released[threadID+"/"+jobID] = n
	}
	return nil
}

// modelPool serves one turn at a time, like the local Ollama.
func modelPool(t *testing.T, model llm.Client) *pool.Pool {
	t.Helper()
	p, err := pool.New([]pool.Endpoint{{Name: "test", Client: model, MaxConcurrency: 1}}, pool.Config{Poll: 5 * time.Millisecond})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	return p
}

func handler(t *testing.T, st *memStore, model llm.Client) (*Handler, *recorder) {
	t.Helper()
	h, rec, _ := handlerWith(t, st, modelPool(t, model))
	return h, rec
}

func handlerWith(t *testing.T, st *memStore, models *pool.Pool) (*Handler, *recorder, *memTurns) {
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
	turns := &memTurns{rec: rec, released: map[string]int{}}
	return NewHandler(Deps{
		Store: st, Publisher: rec, Agent: agent.New(nil, reg, agent.Config{}), Models: models, Turns: turns,
		Users: users{"u-liza": liza}, Directory: directory(),
	}), rec, turns
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

// slow answers after a pause, like a local model under load.
type slow struct{ text string }

func (s slow) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	return s.ChatStream(ctx, req, func(string) {})
}

func (s slow) ChatStream(ctx context.Context, _ llm.Request, onDelta func(string)) (llm.Response, error) {
	select {
	case <-time.After(5 * time.Millisecond):
	case <-ctx.Done():
		return llm.Response{}, ctx.Err()
	}
	onDelta(s.text)
	return llm.Response{Message: llm.Message{Role: llm.RoleAssistant, Content: s.text}}, nil
}

func TestSixTurnsQueueForOneSlot(t *testing.T) {
	st, _ := newStore()
	for i := range 6 {
		id := fmt.Sprint("t", i)
		st.threads[id] = domain.Thread{ID: id, UserID: "u-liza", Type: domain.ThreadGeneral}
		st.messages[id] = []domain.Message{{ID: "q" + id, ThreadID: id, Role: domain.MessageUser, Content: "Привет"}}
	}
	h, rec, _ := handlerWith(t, st, modelPool(t, slow{text: "Привет!"}))

	var wg sync.WaitGroup
	for i := range 6 {
		wg.Go(func() {
			id := fmt.Sprint("t", i)
			if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: id, UserID: "u-liza", MessageID: "q" + id})); err != nil {
				t.Errorf("turn %s: %v", id, err)
			}
		})
	}
	wg.Wait()

	done, queued := 0, 0
	for _, e := range rec.events {
		switch {
		case e.Type == "done":
			done++
		case e.Type == "status" && e.Data["text"] == queuedStatus:
			queued++
		case e.Type == "error":
			t.Errorf("turn failed: %v", e.Data)
		}
	}
	if done != 6 || queued == 0 {
		t.Errorf("done %d of 6, queued %d", done, queued)
	}
}

func TestTurnWaitsForABusyModel(t *testing.T) {
	st, msgID := newStore("Привет")
	models := modelPool(t, fake.New(fake.Text("Привет!")))
	h, rec, _ := handlerWith(t, st, models)
	_, release, err := models.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		release()
	}()

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	if got := rec.types(); !slices.Equal(got, []string{"status", "delta", "done"}) || rec.events[0].Data["text"] != queuedStatus {
		t.Fatalf("published %+v", rec.events)
	}
}

func TestQueueTimeoutFailsTheTurn(t *testing.T) {
	st, msgID := newStore("Привет")
	models := modelPool(t, fake.New())
	h, rec, turns := handlerWith(t, st, models)
	h.d.QueueTimeout = 20 * time.Millisecond
	_, release, err := models.Acquire(context.Background(), nil)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	last := rec.events[len(rec.events)-1]
	if last.Type != "error" || last.Data["code"] != "model_busy" || !strings.Contains(last.Data["message"].(string), "занят") {
		t.Fatalf("last event = %+v", last)
	}
	if len(st.traces) != 1 || !strings.Contains(st.traces[0].Error, "queue") {
		t.Errorf("traces = %+v", st.traces)
	}
	if n, ok := turns.released["t1/"]; !ok || n != len(rec.events)-1 {
		t.Errorf("thread freed at event %d (%v), want before the error", n, ok)
	}
}

func TestThreadIsFreedBeforeDone(t *testing.T) {
	st, msgID := newStore("Привет")
	h, rec, turns := handlerWith(t, st, modelPool(t, fake.New(fake.Text("Привет!"))))

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	if n, ok := turns.released["t1/"]; !ok || n != len(rec.events)-1 || rec.events[n].Type != "done" {
		t.Errorf("thread freed at event %d (%v) of %v", n, ok, rec.types())
	}
}

func TestSkippedTurnFreesTheThread(t *testing.T) {
	st, msgID := newStore("Привет")
	h, _, turns := handlerWith(t, st, modelPool(t, fake.New()))
	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t-none", UserID: "u-liza", MessageID: msgID})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	if _, ok := turns.released["t-none/"]; !ok {
		t.Error("thread of a skipped turn stays locked")
	}
}
