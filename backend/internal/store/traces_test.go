package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func TestSaveAndListTraces(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	liza, _ := s.CreateThread(ctx, "u-liza", domain.ThreadRequest, "", domain.ThreadPin{})
	dmitry, _ := s.CreateThread(ctx, "u-dmitry", domain.ThreadGeneral, "", domain.ThreadPin{})
	q, _ := s.AddMessage(ctx, domain.Message{ThreadID: liza.ID, Role: domain.MessageUser, Content: "Нужен VPN"})
	a, _ := s.AddMessage(ctx, domain.Message{ThreadID: liza.ID, Role: domain.MessageAssistant, Content: "Проверь форму."})

	if err := s.SaveTrace(ctx, domain.TurnTrace{
		ThreadID: liza.ID, QuestionID: q.ID, AnswerID: a.ID, JobID: "j1", Mode: "brief", Duration: 1500 * time.Millisecond,
		SystemPrompt: "# buddy", Steps: json.RawMessage(`[{"tools":[{"name":"prepare_request"}]}]`),
		RawText: "Проверь форму [1].", FinalText: "Проверь форму.", Violations: []string{"unknown ref 1"},
	}); err != nil {
		t.Fatalf("SaveTrace: %v", err)
	}
	if err := s.SaveTrace(ctx, domain.TurnTrace{ThreadID: dmitry.ID, Error: "model: unavailable"}); err != nil {
		t.Fatalf("SaveTrace without messages: %v", err)
	}

	all, err := s.Traces(ctx, TraceFilter{})
	if err != nil || len(all) != 2 || all[0].Error != "model: unavailable" || all[0].Violations == nil {
		t.Fatalf("all = %+v %v", all, err)
	}
	mine, err := s.Traces(ctx, TraceFilter{UserID: "u-liza", Limit: 5})
	if err != nil || len(mine) != 1 {
		t.Fatalf("mine = %+v %v", mine, err)
	}
	tr := mine[0]
	if tr.Question != "Нужен VPN" || tr.ThreadType != domain.ThreadRequest || tr.Duration != 1500*time.Millisecond ||
		tr.Violations[0] != "unknown ref 1" || tr.AnswerID != a.ID || tr.UserID != "u-liza" {
		t.Fatalf("trace = %+v", tr)
	}
	if byThread, _ := s.Traces(ctx, TraceFilter{ThreadID: dmitry.ID}); len(byThread) != 1 || byThread[0].UserID != "u-dmitry" {
		t.Fatalf("by thread = %+v", byThread)
	}
}
