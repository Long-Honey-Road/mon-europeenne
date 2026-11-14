package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/spheres"
)

type fakeSpheres struct {
	ctx   spheres.Context
	err   error
	after []domain.Focus
}

func (f *fakeSpheres) For(context.Context, domain.Thread, domain.Message) (spheres.Context, error) {
	return f.ctx, f.err
}

func (f *fakeSpheres) AfterTurn(_ context.Context, _ domain.Thread, fc domain.Focus) {
	f.after = append(f.after, fc)
}

var access = domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-access"}

func sphereStore(threadType domain.ThreadType) (*memStore, string) {
	st, id := newStore("про ssh", "ответ про ssh", "а что с доступами?")
	th := st.threads["t1"]
	th.Type, th.Summary = threadType, "обзор треда"
	st.threads["t1"] = th
	st.messages["t1"][2].Focus = access
	return st, id
}

// Golden dialogs 1 and 2 of spec 008: the sphere's own history, card and memory replace the thread tail.
func TestTurnAboutASphereSeesOnlyItsContext(t *testing.T) {
	st, msgID := sphereStore(domain.ThreadPlan)
	model := fake.New(fake.Text("По доступам: ждём одобрения."))
	h, _ := handler(t, st, model)
	sp := &fakeSpheres{ctx: spheres.Context{
		Card: "Plan item w1-access «Запросить доступы»", Memory: "Лиза отправила 3 заявки из 7",
		Overview: "- w1-ssh-keys: 2 messages",
		History:  []domain.Message{{Role: domain.MessageUser, Content: "раньше про доступы"}},
	}}
	h.d.Spheres = sp

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID, Mode: "brief"})); err != nil {
		t.Fatal(err)
	}
	call := model.Calls()[0]
	system := call.Messages[0].Content
	for _, want := range []string{"## Sphere in focus", "w1-access", "## Earlier about this sphere", "3 заявки", "## Other spheres in this tab"} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt misses %q", want)
		}
	}
	if strings.Contains(system, "обзор треда") {
		t.Error("thread summary duplicated next to the sphere context")
	}
	var texts []string
	for _, m := range call.Messages[1:] {
		texts = append(texts, m.Content)
	}
	if got := strings.Join(texts, "|"); strings.Contains(got, "про ssh") || !strings.Contains(got, "раньше про доступы") {
		t.Errorf("history = %q, want only the sphere's messages", got)
	}
	if saved := st.messages["t1"][3]; saved.Focus != access || saved.Role != domain.MessageAssistant {
		t.Errorf("answer = %+v, want it on the same sphere", saved)
	}
	if len(sp.after) != 1 || sp.after[0] != access {
		t.Errorf("AfterTurn = %v", sp.after)
	}
}

func TestOverviewWithoutFocusAndPlainThreads(t *testing.T) {
	st, msgID := sphereStore(domain.ThreadPlan)
	st.messages["t1"][2].Focus = domain.Focus{}
	model := fake.New(fake.Text("ok"))
	h, _ := handler(t, st, model)
	h.d.Spheres = &fakeSpheres{ctx: spheres.Context{Overview: "обзор треда\n- w1-access: 3 messages"}}
	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID, Mode: "brief"})); err != nil {
		t.Fatal(err)
	}
	if call := model.Calls()[0]; len(call.Messages) != 4 || !strings.Contains(call.Messages[0].Content, "w1-access: 3 messages") {
		t.Errorf("overview turn = %d messages, prompt %q", len(call.Messages), call.Messages[0].Content)
	}

	// A general thread keeps the thread summary and never asks for a sphere context.
	gen, genID := sphereStore(domain.ThreadGeneral)
	model = fake.New(fake.Text("ok"))
	h, _ = handler(t, gen, model)
	h.d.Spheres = &fakeSpheres{err: errors.New("must not be called")}
	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: genID, Mode: "brief"})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.Calls()[0].Messages[0].Content, "обзор треда") {
		t.Error("general thread lost its summary")
	}
}

func TestSphereContextErrorFailsTheTask(t *testing.T) {
	st, msgID := sphereStore(domain.ThreadPlan)
	h, _ := handler(t, st, fake.New(fake.Text("ok")))
	h.d.Spheres = &fakeSpheres{err: errors.New("db down")}
	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID, Mode: "brief"})); err == nil {
		t.Error("error swallowed")
	}
}

var _ llm.Client = (*fake.Client)(nil)
