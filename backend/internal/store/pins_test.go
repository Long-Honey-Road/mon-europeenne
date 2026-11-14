package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func TestPinsAreTheOwnersOnly(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	th, _ := s.CreateThread(ctx, "u-liza", domain.ThreadPlan, "", domain.ThreadPin{})
	access := domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-access"}
	answer, _ := s.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageAssistant, Content: "Семь доступов", Focus: access})

	p, err := s.CreatePin(ctx, domain.Pin{UserID: "u-liza", Focus: access, Title: "Доступы", Text: "Jira и VPN первыми",
		Sources: json.RawMessage(`[{"kind":"message"}]`), MessageID: answer.ID})
	if err != nil || p.ID == "" || p.MessageID != answer.ID || p.UserID != "u-liza" {
		t.Fatalf("CreatePin = %+v, %v", p, err)
	}
	if _, err := s.CreatePin(ctx, domain.Pin{UserID: "u-liza", Focus: domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-ssh"}, Title: "SSH", Text: "ed25519"}); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.Pins(ctx, "u-liza", domain.Focus{}); len(all) != 2 {
		t.Errorf("all pins = %d", len(all))
	}
	if mine, _ := s.Pins(ctx, "u-liza", access); len(mine) != 1 || mine[0].Title != "Доступы" || string(mine[0].Sources) != `[{"kind": "message"}]` {
		t.Errorf("sphere pins = %+v", mine)
	}
	if others, _ := s.Pins(ctx, "u-anna", domain.Focus{}); len(others) != 0 {
		t.Errorf("another user sees %d pins", len(others))
	}

	up, err := s.UpdatePin(ctx, "u-liza", p.ID, "Доступы, порядок", "Сначала Jira")
	if err != nil || up.Title != "Доступы, порядок" || !up.UpdatedAt.After(up.CreatedAt) && !up.UpdatedAt.Equal(up.CreatedAt) {
		t.Errorf("UpdatePin = %+v, %v", up, err)
	}
	for name, err := range map[string]error{
		"update someone else's": func() error { _, err := s.UpdatePin(ctx, "u-anna", p.ID, "x", "y"); return err }(),
		"update bad id":         func() error { _, err := s.UpdatePin(ctx, "u-liza", "nope", "x", "y"); return err }(),
		"delete someone else's": s.DeletePin(ctx, "u-anna", p.ID),
		"delete bad id":         s.DeletePin(ctx, "u-liza", "nope"),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}

	if m, err := s.OwnMessage(ctx, "u-liza", answer.ID); err != nil || m.Focus != access || m.Content != "Семь доступов" {
		t.Errorf("OwnMessage = %+v, %v", m, err)
	}
	for _, who := range []string{"u-anna"} {
		if _, err := s.OwnMessage(ctx, who, answer.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("OwnMessage by %s: %v", who, err)
		}
	}
	if _, err := s.OwnMessage(ctx, "u-liza", "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("OwnMessage bad id: %v", err)
	}

	if err := s.DeletePin(ctx, "u-liza", p.ID); err != nil {
		t.Fatal(err)
	}
	if mine, _ := s.Pins(ctx, "u-liza", access); len(mine) != 0 {
		t.Errorf("after delete = %+v", mine)
	}
}
