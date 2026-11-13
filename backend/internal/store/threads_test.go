package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

var (
	teams = []domain.Team{{ID: "t-messenger", Name: "Messenger", Unit: "Messenger", Cluster: "Коммуникации"}}
	users = []domain.User{
		{ID: "u-liza", Login: "intern", DisplayName: "Лиза", Role: domain.RoleIntern, Direction: domain.DirectionBackend,
			TeamID: "t-messenger", StartDate: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)},
		{ID: "u-dmitry", Login: "manager", DisplayName: "Дмитрий", Role: domain.RoleManager, TeamID: "t-messenger"},
	}
)

// seeded returns a store over a fresh migrated database with the test team and users.
func seeded(t *testing.T) *Store {
	t.Helper()
	dsn := freshDB(t)
	if err := MigrateUp(dsn); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	pool, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(pool.Close)
	s := New(pool)
	if err := s.SeedOrg(context.Background(), teams, users); err != nil {
		t.Fatalf("SeedOrg: %v", err)
	}
	return s
}

func TestSeedOrgIsIdempotent(t *testing.T) {
	s := seeded(t)
	renamed := []domain.Team{{ID: "t-messenger", Name: "Messenger 2", Cluster: "Коммуникации"}}

	if err := s.SeedOrg(context.Background(), renamed, users); err != nil {
		t.Fatalf("second SeedOrg: %v", err)
	}

	var teamsN, usersN int
	var name string
	ctx := context.Background()
	if err := s.pool.QueryRow(ctx, "SELECT count(*), max(name) FROM buddy.teams").Scan(&teamsN, &name); err != nil {
		t.Fatalf("count teams: %v", err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM buddy.users").Scan(&usersN); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if teamsN != 1 || usersN != 2 || name != "Messenger 2" {
		t.Fatalf("teams %d (%s), users %d", teamsN, name, usersN)
	}
}

func TestSeedOrgRejectsUserOfUnknownTeam(t *testing.T) {
	s := seeded(t)
	ghost := []domain.User{{ID: "u-ghost", Login: "ghost", DisplayName: "x", Role: domain.RoleIntern, TeamID: "t-none"}}
	if err := s.SeedOrg(context.Background(), nil, ghost); err == nil {
		t.Fatal("user in unknown team accepted")
	}
}

func TestThreadsBelongToTheirOwner(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()

	pin := domain.ThreadPin{Task: &domain.Task{Key: "MSG-142", Repo: "messenger-api"}}
	created, err := s.CreateThread(ctx, "u-liza", domain.ThreadTask, "Первая задача", pin)
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	if created.ID == "" || created.UserID != "u-liza" || created.Type != domain.ThreadTask || created.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", created)
	}

	got, err := s.Thread(ctx, "u-liza", created.ID)
	if err != nil || got.Title != "Первая задача" || got.Pin.Task == nil || got.Pin.Task.Key != "MSG-142" {
		t.Fatalf("Thread = %+v, %v", got, err)
	}
	for _, id := range []string{created.ID, "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		if _, err := s.Thread(ctx, "u-dmitry", id); !errors.Is(err, ErrNotFound) && id == created.ID {
			t.Errorf("manager reads intern's thread: %v", err)
		}
		if _, err := s.Thread(ctx, "u-liza", id); id != created.ID && !errors.Is(err, ErrNotFound) {
			t.Errorf("Thread(%q) error = %v, want ErrNotFound", id, err)
		}
	}
}

func TestThreadsListNewestFirst(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	first, _ := s.CreateThread(ctx, "u-liza", domain.ThreadGeneral, "первый", domain.ThreadPin{})
	second, _ := s.CreateThread(ctx, "u-liza", domain.ThreadPlan, "второй", domain.ThreadPin{})
	if _, err := s.CreateThread(ctx, "u-dmitry", domain.ThreadGeneral, "чужой", domain.ThreadPin{}); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	list, err := s.Threads(ctx, "u-liza")
	if err != nil {
		t.Fatalf("Threads: %v", err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("threads = %+v", list)
	}
}

func TestCreateThreadForUnknownUser(t *testing.T) {
	s := seeded(t)
	if _, err := s.CreateThread(context.Background(), "u-ghost", domain.ThreadGeneral, "", domain.ThreadPin{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestMessagesKeepOrderAndLimit(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	th, _ := s.CreateThread(ctx, "u-liza", domain.ThreadGeneral, "", domain.ThreadPin{})

	for i := range 5 {
		role := domain.MessageUser
		if i%2 == 1 {
			role = domain.MessageAssistant
		}
		m := domain.Message{ThreadID: th.ID, Role: role, Content: fmt.Sprint("m", i)}
		if role == domain.MessageAssistant {
			m.Blocks = json.RawMessage(`[{"type":"sources"}]`)
		}
		saved, err := s.AddMessage(ctx, m)
		if err != nil || saved.ID == "" || saved.CreatedAt.IsZero() {
			t.Fatalf("AddMessage: %+v, %v", saved, err)
		}
	}

	last, err := s.Messages(ctx, th.ID, 3)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(last) != 3 || last[0].Content != "m2" || last[2].Content != "m4" {
		t.Fatalf("messages = %+v", last)
	}
	if string(last[1].Blocks) != `[{"type": "sources"}]` || string(last[0].Blocks) != "[]" {
		t.Errorf("blocks = %s / %s", last[1].Blocks, last[0].Blocks)
	}
}

func TestAddMessageToMissingThread(t *testing.T) {
	s := seeded(t)
	_, err := s.AddMessage(context.Background(), domain.Message{ThreadID: "00000000-0000-0000-0000-000000000000", Role: domain.MessageUser})
	if err == nil {
		t.Fatal("message saved into a missing thread")
	}
}

func TestAppendEvents(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()

	err := s.AppendEvents(ctx, "u-liza", []domain.Event{
		{Type: domain.EventMessageSent, Payload: json.RawMessage(`{"thread":"general"}`)},
		{Type: domain.EventToolCalled, Payload: json.RawMessage(`{"tool":"who_to_ask","outcome":"ok"}`)},
	})
	if err != nil {
		t.Fatalf("AppendEvents: %v", err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM buddy.events e JOIN buddy.users u ON u.id = e.actor_id
		WHERE u.external_id = 'u-liza'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("events = %d, %v", n, err)
	}
}

func TestRateMessage(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	th, _ := s.CreateThread(ctx, "u-liza", domain.ThreadGeneral, "", domain.ThreadPin{})
	question, _ := s.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageUser, Content: "?"})
	answer, _ := s.AddMessage(ctx, domain.Message{ThreadID: th.ID, Role: domain.MessageAssistant, Content: "!"})

	if err := s.RateMessage(ctx, "u-liza", answer.ID, "down", "не то"); err != nil {
		t.Fatalf("RateMessage: %v", err)
	}
	var payload map[string]string
	if err := s.pool.QueryRow(ctx, `SELECT payload FROM buddy.events WHERE type_id = $1`, int16(domain.EventFeedback)).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["message_id"] != answer.ID || payload["value"] != "down" || payload["note"] != "не то" || payload["thread_id"] != th.ID {
		t.Fatalf("payload = %v", payload)
	}

	for name, tc := range map[string][2]string{
		"own question":    {"u-liza", question.ID},
		"someone else's":  {"u-dmitry", answer.ID},
		"unknown message": {"u-liza", "00000000-0000-0000-0000-000000000000"},
		"malformed id":    {"u-liza", "nope"},
	} {
		if err := s.RateMessage(ctx, tc[0], tc[1], "up", ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
