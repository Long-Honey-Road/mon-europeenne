package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func TestFocusMessagesSummariesAndActivity(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	th, err := s.CreateThread(ctx, "u-liza", domain.ThreadPlan, "", domain.ThreadPin{})
	if err != nil {
		t.Fatal(err)
	}
	access := domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-access"}
	ssh := domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-ssh-keys"}
	var saved []domain.Message
	for i, f := range []domain.Focus{access, access, ssh, {}, access} {
		m, err := s.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageUser, Content: string(rune('a' + i)), Focus: f})
		if err != nil {
			t.Fatal(err)
		}
		saved = append(saved, m)
		time.Sleep(2 * time.Millisecond)
	}

	all, err := s.Messages(ctx, th.ID, 10)
	if err != nil || len(all) != 5 || all[0].Focus != access || !all[3].Focus.IsZero() {
		t.Fatalf("Messages = %+v, %v", all, err)
	}
	got, err := s.FocusMessages(ctx, th.ID, access, saved[4].CreatedAt, 8)
	if err != nil || len(got) != 2 || got[0].Content != "a" || got[1].Content != "b" {
		t.Errorf("FocusMessages before the last = %+v, %v", got, err)
	}
	if got, _ := s.FocusMessages(ctx, th.ID, access, time.Now().Add(time.Hour), 1); len(got) != 1 || got[0].Content != "e" {
		t.Errorf("limit keeps the newest: %+v", got)
	}

	foci, err := s.ThreadFoci(ctx, th.ID)
	if err != nil || len(foci) != 2 || foci[0].Focus != access || foci[0].Messages != 3 || foci[1].Messages != 1 {
		t.Errorf("ThreadFoci = %+v, %v", foci, err)
	}
	if n, _ := s.CountAfter(ctx, th.ID, access, saved[0].CreatedAt); n != 2 {
		t.Errorf("CountAfter(access) = %d, want 2", n)
	}
	if n, _ := s.CountAfter(ctx, th.ID, domain.Focus{}, time.Time{}); n != 5 {
		t.Errorf("CountAfter(thread) = %d, want 5", n)
	}

	if sum, until, err := s.FocusSummary(ctx, th.ID, access); err != nil || sum != "" || !until.IsZero() {
		t.Errorf("empty summary = %q, %v, %v", sum, until, err)
	}
	for _, text := range []string{"первое", "второе"} {
		if err := s.SaveFocusSummary(ctx, th.ID, access, text, saved[1].CreatedAt); err != nil {
			t.Fatal(err)
		}
	}
	if sum, until, _ := s.FocusSummary(ctx, th.ID, access); sum != "второе" || !until.Equal(saved[1].CreatedAt) {
		t.Errorf("summary = %q until %v", sum, until)
	}

	if err := s.SaveThreadSummary(ctx, th.ID, "обзор", saved[4].CreatedAt); err != nil {
		t.Fatal(err)
	}
	back, _ := s.Thread(ctx, "u-liza", th.ID)
	if back.Summary != "обзор" || !back.SummaryUntil.Equal(saved[4].CreatedAt) {
		t.Errorf("thread = %+v", back)
	}
	if err := s.SaveThreadSummary(ctx, "00000000-0000-0000-0000-000000000000", "x", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing thread: %v", err)
	}
	for _, bad := range []error{
		func() error { _, err := s.FocusMessages(ctx, "nope", access, time.Now(), 1); return err }(),
		func() error { _, err := s.ThreadFoci(ctx, "nope"); return err }(),
	} {
		if !errors.Is(bad, ErrNotFound) {
			t.Errorf("bad thread id: %v", bad)
		}
	}
}
