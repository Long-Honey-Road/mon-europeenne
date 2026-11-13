package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
)

func generalRegistry(t *testing.T) *tools.Registry {
	t.Helper()
	gk, err := tools.GeneralKnowledge()
	if err != nil {
		t.Fatal(err)
	}
	r, err := tools.NewRegistry(gk)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestGeneralKnowledgeIsMarkedByTheServer(t *testing.T) {
	model := fake.New(
		fake.ToolCall("general_knowledge", map[string]string{"topic": "подключение по SSH"}),
		fake.Text("В документах команды этого нет, спроси Анну. В общем случае SSH подключается так: ssh user@host."),
	)
	res, err := New(model, generalRegistry(t), Config{}).Run(context.Background(), turn("Как подключиться по ssh к кластеру?"), &recorder{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Blocks) != 1 || res.Blocks[0].Type != "general_knowledge" || res.Blocks[0].Topic != "подключение по SSH" {
		t.Fatalf("blocks = %+v", res.Blocks)
	}
	if got := eventTypes(res); got[len(got)-1] != domain.EventGeneralKnowledge {
		t.Fatalf("events = %v", got)
	}
	// The tool result reminds the model of the rules at the moment it answers.
	if msg := lastToolMessage(t, model); !strings.Contains(msg["rules"].(string), "Never name Avito hosts") {
		t.Fatalf("tool result = %v", msg)
	}
}

func TestDetailedModeUsesItsReasoningEffort(t *testing.T) {
	model := fake.New(fake.Text("коротко"), fake.Text("подробно"))
	a := New(model, generalRegistry(t), Config{DetailedEffort: "low"})

	brief := turn("вопрос")
	if _, err := a.Run(context.Background(), brief, &recorder{}); err != nil {
		t.Fatal(err)
	}
	detailed := turn("вопрос")
	detailed.Mode = ModeDetailed
	if _, err := a.Run(context.Background(), detailed, &recorder{}); err != nil {
		t.Fatal(err)
	}
	calls := model.Calls()
	if calls[0].ReasoningEffort != "" || calls[1].ReasoningEffort != "low" {
		t.Fatalf("efforts = %q, %q", calls[0].ReasoningEffort, calls[1].ReasoningEffort)
	}
}

func TestTodayLineAndPrompt(t *testing.T) {
	start := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	// 22:30 UTC on Thursday is already Friday in Moscow.
	now := time.Date(2026, 11, 12, 22, 30, 0, 0, time.UTC)
	if got := TodayLine(now, start); got != "2026-11-13, пятница; день стажировки 5" {
		t.Fatalf("line = %q", got)
	}
	if got := TodayLine(now, time.Time{}); got != "2026-11-13, пятница" {
		t.Fatalf("no start = %q", got)
	}
	if got := TodayLine(now, start.AddDate(0, 0, 10)); !strings.Contains(got, "стажировка начнётся 2026-11-19") {
		t.Fatalf("before start = %q", got)
	}

	tr := turn("привет")
	tr.Today = "2026-11-13, пятница; день стажировки 5"
	prompt, err := systemPrompt(tr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "## Today\n\n2026-11-13, пятница; день стажировки 5") || !strings.Contains(prompt, "Your name is buddy") {
		t.Fatalf("prompt misses today or name:\n%s", prompt)
	}
}
