package store

import (
	"context"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func TestFeedbackListsRatingsWithTheirQuestions(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	th, _ := s.CreateThread(ctx, "u-liza", domain.ThreadGeneral, "", domain.ThreadPin{})
	_, _ = s.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageUser, Content: "Кто поможет с деплоем?"})
	a1, _ := s.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageAssistant, Content: "Игорь."})
	_, _ = s.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageUser, Content: "А с VPN?"})
	a2, _ := s.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageAssistant, Content: "Форма открыта слева."})

	for _, r := range []struct{ id, value, note string }{
		{a1.ID, "up", ""}, {a2.ID, "down", ""}, {a2.ID, "down", "выдумал форму"},
	} {
		if err := s.RateMessage(ctx, "u-liza", r.id, r.value, r.note); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.Feedback(ctx, 10, false)
	if err != nil || len(all) != 3 {
		t.Fatalf("feedback = %+v, %v", all, err)
	}
	last := all[0]
	if last.Value != "down" || last.Note != "выдумал форму" || last.Question != "А с VPN?" || last.Answer != "Форма открыта слева." ||
		last.Login != "intern" || last.ThreadID != th.ID || last.At.IsZero() {
		t.Errorf("newest = %+v", last)
	}
	if all[2].Question != "Кто поможет с деплоем?" || all[2].Value != "up" {
		t.Errorf("oldest = %+v", all[2])
	}
	notes, err := s.Feedback(ctx, 10, true)
	if err != nil || len(notes) != 1 || notes[0].Note != "выдумал форму" {
		t.Errorf("only notes = %+v, %v", notes, err)
	}
	if one, _ := s.Feedback(ctx, 1, false); len(one) != 1 {
		t.Errorf("limit 1 gave %d", len(one))
	}
}
