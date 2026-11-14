package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/agent"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

func TestTracePrintsTurns(t *testing.T) {
	cfg := stackConfig(t)
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	st := store.New(pool)
	th, _ := st.CreateThread(ctx, "u-liza", domain.ThreadGeneral, "", domain.ThreadPin{})
	q, _ := st.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageUser, Content: "Кто такой Тимур?"})
	steps, _ := json.Marshal([]agent.TraceStep{
		{DurationMS: 900, PromptTokens: 1800, CompletionTokens: 20, Text: "Сейчас посмотрю",
			Tools: []agent.TraceTool{{Name: "who_to_ask", Args: json.RawMessage(`{"topic":"Тимур"}`), Outcome: "ok", Result: `{"people":[]}`}}},
		{DurationMS: 700, ReasoningEffort: "low"},
	})
	if err := st.SaveTrace(ctx, domain.TurnTrace{ThreadID: th.ID, QuestionID: q.ID, Mode: "brief", Duration: 1600 * time.Millisecond,
		SystemPrompt: "# buddy prompt", Steps: steps, RawText: "Не нашёл [1].", FinalText: "Не нашёл.", Violations: []string{"unknown ref 1"}}); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := run([]string{"trace", "-as", "intern", "-prompt"}, &out, &errOut); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	for _, want := range []string{"Q: Кто такой Тимур?", "withdrawn text: Сейчас посмотрю", `→ who_to_ask {"topic":"Тимур"} [ok`,
		`← {"people":[]}`, "effort low", "raw:   Не нашёл [1].", "final: Не нашёл.", "violations: unknown ref 1", "# buddy prompt"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output misses %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if code := run([]string{"trace", "-json", "-thread", th.ID}, &out, &errOut); code != exitOK || !strings.Contains(out.String(), `"FinalText":"Не нашёл."`) {
		t.Fatalf("json: %d %s", code, out.String())
	}
	out.Reset()
	if code := run([]string{"trace", "-as", "manager"}, &out, &errOut); code != exitOK || out.Len() != 0 {
		t.Fatalf("other user: %d %q", code, out.String())
	}
	for _, args := range [][]string{{"trace", "-as", "ghost"}, {"trace", "-nope"}} {
		if code := run(args, &out, &errOut); code != exitError {
			t.Errorf("%v: exit %d", args, code)
		}
	}
	t.Setenv("DATABASE_URL", "")
	if code := run([]string{"trace"}, &out, &errOut); code != exitError {
		t.Errorf("without database: exit %d", code)
	}
}

func TestFeedbackPrintsRatings(t *testing.T) {
	cfg := stackConfig(t)
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	st := store.New(pool)
	th, _ := st.CreateThread(ctx, "u-liza", domain.ThreadGeneral, "", domain.ThreadPin{})
	_, _ = st.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageUser, Content: "Нужен VPN"})
	a, _ := st.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageAssistant, Content: "Форма\nслева."})
	_ = st.RateMessage(ctx, "u-liza", a.ID, "down", "выдумал форму")

	var out, errOut bytes.Buffer
	if code := run([]string{"feedback", "-notes"}, &out, &errOut); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	for _, want := range []string{"· intern · 👎 · thread " + th.ID, "Q: Нужен VPN", "A: Форма слева.", "note: выдумал форму"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output misses %q:\n%s", want, out.String())
		}
	}
	if code := run([]string{"feedback", "-nope"}, &out, &errOut); code != exitError {
		t.Errorf("bad flag: exit %d", code)
	}
	t.Setenv("DATABASE_URL", "")
	if code := run([]string{"feedback"}, &out, &errOut); code != exitError {
		t.Errorf("no database: exit %d", code)
	}
}
