package jobs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/llm/fake"
)

func TestFileRulesReadsTheFileEveryTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.md")
	rules := FileRules(path)
	if got := rules(); got != "" {
		t.Errorf("missing file gave %q", got)
	}
	if err := os.WriteFile(path, []byte("\n Напоминай про отзывы.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := rules(); got != "Напоминай про отзывы." {
		t.Errorf("rules = %q", got)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("я", maxStandRules)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := rules(); len(got) > maxStandRules {
		t.Errorf("rules not capped: %d bytes", len(got))
	}
	if got := FileRules(t.TempDir())(); got != "" {
		t.Errorf("a directory gave %q", got)
	}
}

func TestStandRulesReachTheAgent(t *testing.T) {
	st, msgID := newStore("Привет")
	model := fake.New(fake.Text("Привет!"))
	h, _ := handler(t, st, model)
	h.d.StandRules = func() string { return "Это стейджинг." }

	if err := h.ProcessTask(context.Background(), task(t, ChatTurn{ThreadID: "t1", UserID: "u-liza", MessageID: msgID, Mode: "brief"})); err != nil {
		t.Fatalf("ProcessTask: %v", err)
	}
	if system := model.Calls()[0].Messages[0].Content; !strings.HasSuffix(system, "Это стейджинг.") {
		t.Errorf("rules missing from the prompt: %q", system[max(0, len(system)-200):])
	}
}
