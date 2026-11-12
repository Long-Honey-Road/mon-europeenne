package corpsim

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/requests"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/testredis"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

func TestMain(m *testing.M) { os.Exit(testredis.Main(m)) }

var t0 = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

func newSim(t *testing.T, outcomes map[string]Outcome) *Sim {
	t.Helper()
	c := redis.NewClient(&redis.Options{Addr: testredis.Addr(t)})
	t.Cleanup(func() { _ = c.Close() })
	s := New(c, outcomes, "test:"+t.Name())
	s.now = func() time.Time { return t0 }
	return s
}

func submit(t *testing.T, s *Sim, typeKey string, approvers ...string) requests.Ticket {
	t.Helper()
	var chain []requests.Approver
	for _, a := range approvers {
		chain = append(chain, requests.Approver{Role: "object_owner", Name: a})
	}
	tk, err := s.Submit(context.Background(), requests.Submission{TypeKey: typeKey, RequesterID: "u-liza",
		Fields: map[string]any{"channel": "~messenger-team"}, Approvers: chain})
	if err != nil {
		t.Fatal(err)
	}
	return tk
}

func TestSubmitThenApproveWhenDue(t *testing.T) {
	s := newSim(t, map[string]Outcome{"access.mm": {Approve: true, After: 15 * time.Second}})
	ctx := context.Background()

	tk := submit(t, s, "access.mm", "Дмитрий")
	if !strings.HasPrefix(tk.ID, "REQ-") || tk.Status != requests.StatusPending || tk.Approver().Name != "Дмитрий" {
		t.Fatalf("ticket = %+v", tk)
	}
	if next := submit(t, s, "access.mm", "Дмитрий"); next.ID == tk.ID {
		t.Fatal("ids must be unique")
	}

	if changed, err := s.Advance(ctx, t0.Add(10*time.Second)); err != nil || len(changed) != 0 {
		t.Fatalf("too early: %v %v", changed, err)
	}
	changed, err := s.Advance(ctx, t0.Add(15*time.Second))
	if err != nil || len(changed) != 2 {
		t.Fatalf("due: %v %v", changed, err)
	}
	got := changed[0]
	if got.Status != requests.StatusApproved || len(got.History) != 2 || got.History[1].By != "Дмитрий" {
		t.Fatalf("approved = %+v", got)
	}
	if again, _ := s.Advance(ctx, t0.Add(time.Hour)); len(again) != 0 {
		t.Fatalf("decided requests must not change again: %v", again)
	}
}

func TestRejectCarriesTheReason(t *testing.T) {
	s := newSim(t, map[string]Outcome{"access.sentry": {Approve: false, After: time.Second, Reason: "слишком общее"}})
	submit(t, s, "access.sentry", "Анна")

	changed, err := s.Advance(context.Background(), t0.Add(time.Minute))
	if err != nil || len(changed) != 1 {
		t.Fatalf("%v %v", changed, err)
	}
	if c := changed[0]; c.Status != requests.StatusRejected || c.Reason != "слишком общее" || c.History[1].Note != "слишком общее" {
		t.Fatalf("rejected = %+v", c)
	}
}

func TestMultiStepChainGoesApproverByApprover(t *testing.T) {
	s := newSim(t, map[string]Outcome{"access.x": {Approve: true, After: 10 * time.Second}})
	ctx := context.Background()
	submit(t, s, "access.x", "Дмитрий", "Игорь")

	first, _ := s.Advance(ctx, t0.Add(10*time.Second))
	if len(first) != 1 || first[0].Status != requests.StatusPending || first[0].Approver().Name != "Игорь" {
		t.Fatalf("after first step: %+v", first)
	}
	if early, _ := s.Advance(ctx, t0.Add(15*time.Second)); len(early) != 0 {
		t.Fatalf("second step is scheduled anew: %+v", early)
	}
	second, _ := s.Advance(ctx, t0.Add(20*time.Second))
	if len(second) != 1 || second[0].Status != requests.StatusApproved || len(second[0].History) != 3 {
		t.Fatalf("after second step: %+v", second)
	}
}

func TestUnknownTypeIsApprovedAfterDefaultDelay(t *testing.T) {
	s := newSim(t, nil)
	tk := submit(t, s, "access.unknown")
	if tk.Approvers == nil || tk.Approver() != (requests.Approver{}) {
		t.Fatalf("no chain: %+v", tk)
	}
	if c, _ := s.Advance(context.Background(), t0.Add(defaultDelay)); len(c) != 1 || c[0].Status != requests.StatusApproved {
		t.Fatalf("%+v", c)
	}
}

func TestOutcomesFromSeed(t *testing.T) {
	data, err := seed.Load(seed.Files)
	if err != nil {
		t.Fatal(err)
	}
	o := OutcomesFromSeed(data)
	if sentry := o["access.sentry_project"]; sentry.Approve || sentry.Reason == "" {
		t.Fatalf("sentry = %+v", sentry)
	}
	if vpn := o["access.vpn"]; !vpn.Approve || vpn.After != time.Minute {
		t.Fatalf("vpn = %+v", vpn)
	}
}
