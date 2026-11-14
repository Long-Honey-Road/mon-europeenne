package spheres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

var (
	t0     = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	access = domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-access"}
	ssh    = domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-ssh-keys"}
)

type memStore struct {
	msgs      []domain.Message
	summaries map[domain.Focus]string
	until     map[domain.Focus]time.Time
	err       error
}

func newMem() *memStore {
	m := &memStore{summaries: map[domain.Focus]string{}, until: map[domain.Focus]time.Time{}}
	for i, f := range []domain.Focus{access, access, ssh, access, {}} {
		role := domain.MessageUser
		if i%2 == 1 {
			role = domain.MessageAssistant
		}
		m.msgs = append(m.msgs, domain.Message{ID: string(rune('a' + i)), Role: role, Content: "msg " + string(rune('a'+i)), Focus: f, CreatedAt: t0.Add(time.Duration(i) * time.Minute)})
	}
	return m
}

func (m *memStore) FocusMessages(_ context.Context, _ string, f domain.Focus, before time.Time, limit int) ([]domain.Message, error) {
	var out []domain.Message
	for _, msg := range m.msgs {
		if msg.Focus == f && msg.CreatedAt.Before(before) {
			out = append(out, msg)
		}
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, m.err
}

func (m *memStore) Messages(context.Context, string, int) ([]domain.Message, error) {
	return m.msgs, m.err
}

func (m *memStore) ThreadFoci(context.Context, string) ([]store.FocusActivity, error) {
	return []store.FocusActivity{{Focus: access, Messages: 3, Last: t0}, {Focus: ssh, Messages: 1, Last: t0}}, m.err
}

func (m *memStore) FocusSummary(_ context.Context, _ string, f domain.Focus) (string, time.Time, error) {
	return m.summaries[f], m.until[f], m.err
}

func (m *memStore) ThreadSummary(context.Context, string) (string, time.Time, error) {
	return m.summaries[domain.Focus{}], m.until[domain.Focus{}], m.err
}

func (m *memStore) CountAfter(_ context.Context, _ string, f domain.Focus, after time.Time) (int, error) {
	n := 0
	for _, msg := range m.msgs {
		if msg.CreatedAt.After(after) && (f.IsZero() || msg.Focus == f) {
			n++
		}
	}
	return n * 3, m.err
}

func (m *memStore) SaveFocusSummary(_ context.Context, _ string, f domain.Focus, s string, until time.Time) error {
	m.summaries[f], m.until[f] = s, until
	return m.err
}

func (m *memStore) SaveThreadSummary(_ context.Context, _ string, s string, until time.Time) error {
	m.summaries[domain.Focus{}], m.until[domain.Focus{}] = s, until
	return m.err
}

type plans struct{ err error }

func (p plans) Plan(context.Context, string) ([]domain.PlanEntry, error) {
	return []domain.PlanEntry{
		{Item: domain.PlanItem{Key: "w1-intro", Title: "Познакомиться", Period: domain.PeriodWeek1}, Status: domain.ItemDoing},
		{Item: domain.PlanItem{Key: "w1-access", Title: "Запросить доступы", Practice: "Запроси семь доступов", Period: domain.PeriodWeek1, DependsOn: []string{"w1-intro"}}, Status: domain.ItemDoing, Note: "3 из 7"},
		{Item: domain.PlanItem{Key: "w1-ssh-keys", Title: "SSH-ключ", Period: domain.PeriodWeek1}, Status: domain.ItemTodo},
	}, p.err
}

type pinsOf map[domain.Focus][]domain.Pin

func (p pinsOf) Pins(_ context.Context, userID string, f domain.Focus) ([]domain.Pin, error) {
	if userID == "u-broken" {
		return nil, errors.New("db down")
	}
	return p[f], nil
}

type vaultOf map[string]domain.VaultItem

func (v vaultOf) VaultItem(_ context.Context, _ string, id string) (domain.VaultItem, error) {
	item, ok := v[id]
	if !ok {
		return domain.VaultItem{}, store.ErrNotFound
	}
	return item, nil
}

type queue struct{ got []domain.Focus }

func (q *queue) EnqueueSummary(_ context.Context, _ string, f domain.Focus) error {
	q.got = append(q.got, f)
	return nil
}

func TestForAFocusedMessage(t *testing.T) {
	st := newMem()
	st.summaries[access] = "Лиза отправила три заявки"
	svc := New(st, plans{}, pinsOf{}, vaultOf{}, &queue{})
	th := domain.Thread{ID: "t1", UserID: "u-liza", Summary: "обзор"}

	c, err := svc.For(t.Context(), th, st.msgs[3])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Card, "«Запросить доступы»") || !strings.Contains(c.Card, "Practice: Запроси") || !strings.Contains(c.Card, "Note: 3 из 7") || !strings.Contains(c.Card, "w1-intro «Познакомиться»") {
		t.Errorf("card = %q", c.Card)
	}
	if c.Memory != "Лиза отправила три заявки" {
		t.Errorf("memory = %q", c.Memory)
	}
	if len(c.History) != 2 || c.History[0].ID != "a" || c.History[1].ID != "b" {
		t.Errorf("history = %+v, want the two earlier w1-access messages", c.History)
	}
	if !strings.HasPrefix(c.Overview, "обзор\n") || !strings.Contains(c.Overview, "w1-ssh-keys «SSH-ключ», todo: 1 messages") || strings.Contains(c.Overview, "w1-access") {
		t.Errorf("overview = %q", c.Overview)
	}

	plain, err := svc.For(t.Context(), th, st.msgs[4])
	if err != nil || plain.Card != "" || plain.History != nil || !strings.Contains(plain.Overview, "w1-access «Запросить доступы»") {
		t.Errorf("overview turn = %+v, %v", plain, err)
	}
	unknown := domain.Message{Focus: domain.Focus{Kind: domain.FocusPlanItem, ID: "ghost"}, CreatedAt: t0}
	if c, _ := svc.For(t.Context(), th, unknown); c.Card != "plan_item ghost" {
		t.Errorf("unknown sphere card = %q", c.Card)
	}
}

func TestVaultRecordCard(t *testing.T) {
	rec := domain.Focus{Kind: domain.FocusVaultItem, ID: "v1"}
	st := newMem()
	st.msgs = append(st.msgs, domain.Message{ID: "z", Focus: rec, CreatedAt: t0.Add(time.Hour)})
	vault := vaultOf{"v1": {ID: "v1", Title: "Доступы", Version: 3, Tags: []string{"w1"}, Summary: "Jira первой"}}
	svc := New(st, plans{}, pinsOf{}, vault, &queue{})
	th := domain.Thread{ID: "t1", UserID: "u-liza"}
	c, err := svc.For(t.Context(), th, domain.Message{Focus: rec, CreatedAt: t0.Add(2 * time.Hour)})
	if err != nil || !strings.Contains(c.Card, "«Доступы», version 3, tags w1") || !strings.Contains(c.Card, "Jira первой") {
		t.Errorf("card = %q, %v", c.Card, err)
	}
	if _, err := svc.For(t.Context(), th, domain.Message{Focus: domain.Focus{Kind: domain.FocusVaultItem, ID: "nope"}}); err == nil {
		t.Error("missing record accepted")
	}
}

func TestCardListsPins(t *testing.T) {
	var many []domain.Pin
	for i := range 12 {
		many = append(many, domain.Pin{Title: fmt.Sprint("закреп ", i), Text: strings.Repeat("я", 300) + "\nконец"})
	}
	svc := New(newMem(), plans{}, pinsOf{access: many}, vaultOf{}, &queue{})
	c, err := svc.For(t.Context(), domain.Thread{ID: "t1", UserID: "u-liza"}, domain.Message{Focus: access, CreatedAt: t0.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(c.Card, "\n- закреп "); n != CardPins || strings.Contains(c.Card, "конец") || !strings.Contains(c.Card, "Pinned by the user:") {
		t.Errorf("card has %d pins: %q", n, c.Card)
	}
	if _, err := svc.For(t.Context(), domain.Thread{ID: "t1", UserID: "u-broken"}, domain.Message{Focus: access}); err == nil {
		t.Error("pin error swallowed")
	}
}

func TestForErrors(t *testing.T) {
	th := domain.Thread{ID: "t1", UserID: "u-liza"}
	if _, err := New(newMem(), plans{err: errors.New("down")}, pinsOf{}, vaultOf{}, &queue{}).For(t.Context(), th, domain.Message{}); err == nil {
		t.Error("plan error swallowed")
	}
	st := newMem()
	st.err = errors.New("db down")
	if _, err := New(st, plans{}, pinsOf{}, vaultOf{}, &queue{}).For(t.Context(), th, domain.Message{Focus: access}); err == nil {
		t.Error("store error swallowed")
	}
}

func TestAfterTurnSchedulesSummaries(t *testing.T) {
	st := newMem()
	q := &queue{}
	svc := New(st, plans{}, pinsOf{}, vaultOf{}, q)
	th := domain.Thread{ID: "t1"}
	svc.AfterTurn(t.Context(), th, access)
	if len(q.got) != 2 || q.got[0] != access || !q.got[1].IsZero() {
		t.Errorf("scheduled %v, want the sphere and the thread", q.got)
	}

	q.got = nil
	st.until[access] = t0.Add(time.Hour)
	th.SummaryUntil = t0.Add(time.Hour)
	svc.AfterTurn(t.Context(), th, access)
	if len(q.got) != 0 {
		t.Errorf("nothing new, scheduled %v", q.got)
	}
	st.err = errors.New("db down")
	svc.AfterTurn(t.Context(), th, domain.Focus{})
}

type models struct {
	client llm.Client
	err    error
}

func (m models) Acquire(context.Context, func()) (llm.Client, func(), error) {
	return m.client, func() {}, m.err
}

func TestSummarizeSphereAndThread(t *testing.T) {
	st := newMem()
	st.summaries[access], st.until[access] = "старое", t0
	model := fake.New(fake.Text("  Лиза отправила заявки, ждёт одобрения.  "), fake.Text(strings.Repeat("я", 1500)))
	s := NewSummarizer(st, models{client: model})

	if err := s.Summarize(t.Context(), "t1", access); err != nil {
		t.Fatal(err)
	}
	if st.summaries[access] != "Лиза отправила заявки, ждёт одобрения." || !st.until[access].Equal(st.msgs[3].CreatedAt) {
		t.Errorf("summary = %q until %v", st.summaries[access], st.until[access])
	}
	prompt := model.Calls()[0].Messages[1].Content
	if !strings.Contains(prompt, "старое") || strings.Contains(prompt, "msg a") || !strings.Contains(prompt, "buddy: msg b") || !strings.Contains(prompt, "buddy: msg d") {
		t.Errorf("prompt = %q, want the old summary and only newer messages", prompt)
	}

	if err := s.Summarize(t.Context(), "t1", domain.Focus{}); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(st.summaries[domain.Focus{}])); n != maxSummaryRunes || !st.until[domain.Focus{}].Equal(st.msgs[4].CreatedAt) {
		t.Errorf("thread summary %d runes until %v", n, st.until[domain.Focus{}])
	}

	// Nothing new: no model call.
	if err := s.Summarize(t.Context(), "t1", access); err != nil || len(model.Calls()) != 2 {
		t.Errorf("repeat = %v, calls %d", err, len(model.Calls()))
	}
}

func TestSummarizeFailures(t *testing.T) {
	st := newMem()
	if err := NewSummarizer(st, models{err: errors.New("busy")}).Summarize(t.Context(), "t1", access); err == nil {
		t.Error("busy model accepted")
	}
	if err := NewSummarizer(st, models{client: fake.New(fake.Text("   "))}).Summarize(t.Context(), "t1", access); err != nil || st.summaries[access] != "" {
		t.Errorf("empty answer = %v, saved %q", err, st.summaries[access])
	}
	if err := NewSummarizer(st, models{client: fake.New()}).Summarize(t.Context(), "t1", access); err == nil {
		t.Error("model error swallowed")
	}
	st.err = errors.New("db down")
	if err := NewSummarizer(st, models{client: fake.New()}).Summarize(t.Context(), "t1", access); err == nil {
		t.Error("store error swallowed")
	}
}
