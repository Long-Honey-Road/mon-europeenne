package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

var planItems = []domain.PlanItem{
	{Key: "a", Title: "Для всех", Period: domain.PeriodWeek1, Map: domain.MapPosition{X: 0, Y: 0, Group: "team"}},
	{Key: "b", Title: "Backend", Period: domain.PeriodWeek1, Direction: domain.DirectionBackend, DependsOn: []string{"a"}},
	{Key: "c", Title: "Frontend", Period: domain.PeriodWeek2, Direction: domain.DirectionFrontend},
	{Key: "d", Title: "Другая команда", Period: domain.PeriodWeek2, TeamID: "t-other"},
	{Key: "e", Title: "Своя команда", Period: domain.PeriodMonth1, TeamID: "t-messenger", Sources: []string{"doc-1"}},
}

func seededPlan(t *testing.T) *Store {
	t.Helper()
	s := seeded(t)
	ctx := context.Background()
	if err := s.SeedOrg(ctx, []domain.Team{{ID: "t-other", Name: "Other"}}, nil); err != nil {
		t.Fatalf("SeedOrg: %v", err)
	}
	statuses := []domain.PlanStatus{{UserID: "u-liza", ItemID: "a", Status: domain.ItemDone, Note: "познакомилась"}}
	if err := s.SeedPlan(ctx, planItems, statuses); err != nil {
		t.Fatalf("SeedPlan: %v", err)
	}
	return s
}

func keys(entries []domain.PlanEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Item.Key)
	}
	return out
}

func TestPlanShowsOnlyItemsForTheUser(t *testing.T) {
	s := seededPlan(t)

	plan, err := s.Plan(context.Background(), "u-liza")
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if got := keys(plan); !slices.Equal(got, []string{"a", "b", "e"}) {
		t.Fatalf("plan = %v, want items for everyone, backend and her team", got)
	}
	a, b := plan[0], plan[1]
	if a.Status != domain.ItemDone || a.Note != "познакомилась" || a.Item.Map.Group != "team" {
		t.Errorf("a = %+v", a)
	}
	if b.Status != domain.ItemTodo || !slices.Equal(b.Item.DependsOn, []string{"a"}) || b.Item.Direction != domain.DirectionBackend {
		t.Errorf("b = %+v", b)
	}
	if !slices.Equal(plan[2].Item.Sources, []string{"doc-1"}) {
		t.Errorf("e sources = %v", plan[2].Item.Sources)
	}
}

func TestSetItemStatus(t *testing.T) {
	s := seededPlan(t)
	ctx := context.Background()

	if err := s.SetItemStatus(ctx, "u-liza", "b", domain.ItemDoing, "начала"); err != nil {
		t.Fatalf("SetItemStatus: %v", err)
	}
	plan, _ := s.Plan(ctx, "u-liza")
	if plan[1].Status != domain.ItemDoing || plan[1].Note != "начала" {
		t.Fatalf("b = %+v", plan[1])
	}
	manager, _ := s.Plan(ctx, "u-dmitry")
	for _, e := range manager {
		if e.Item.Key == "b" {
			t.Fatal("manager sees a backend item though he has no direction")
		}
	}
}

func TestSetItemStatusOutsideThePlan(t *testing.T) {
	s := seededPlan(t)
	for _, key := range []string{"c", "d", "zzz"} {
		if err := s.SetItemStatus(context.Background(), "u-liza", key, domain.ItemDone, ""); !errors.Is(err, ErrNotFound) {
			t.Errorf("SetItemStatus(%s) error = %v, want ErrNotFound", key, err)
		}
	}
}

func TestReseedKeepsProgressAndUpdatesTexts(t *testing.T) {
	s := seededPlan(t)
	ctx := context.Background()
	if err := s.SetItemStatus(ctx, "u-liza", "a", domain.ItemDoing, "заново"); err != nil {
		t.Fatalf("SetItemStatus: %v", err)
	}
	renamed := append([]domain.PlanItem(nil), planItems...)
	renamed[0].Title = "Для всех, v2"
	renamed[1].DependsOn = nil

	if err := s.SeedPlan(ctx, renamed, []domain.PlanStatus{{UserID: "u-liza", ItemID: "a", Status: domain.ItemDone}}); err != nil {
		t.Fatalf("SeedPlan again: %v", err)
	}

	plan, _ := s.Plan(ctx, "u-liza")
	if plan[0].Item.Title != "Для всех, v2" || plan[0].Status != domain.ItemDoing || len(plan[1].Item.DependsOn) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestSeedPlanRejectsUnknownDependency(t *testing.T) {
	s := seeded(t)
	bad := []domain.PlanItem{{Key: "x", Title: "x", Period: domain.PeriodWeek1, DependsOn: []string{"nope"}}}
	if err := s.SeedPlan(context.Background(), bad, nil); err == nil {
		t.Fatal("unknown dependency accepted")
	}
}

func TestEnsureUserGivesAHiredInternTheSharedPlan(t *testing.T) {
	s := seededPlan(t)
	ctx := context.Background()
	vera := domain.User{
		ID: "u-vnovikova", Login: "vnovikova", DisplayName: "Вера", Role: domain.RoleIntern,
		Direction: domain.DirectionBackend, TeamID: "t-messenger", StartDate: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC),
	}
	if _, err := s.Plan(ctx, vera.ID); err != nil {
		t.Fatalf("Plan before the user exists: %v", err)
	}
	if err := s.EnsureUser(ctx, vera); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureUser(ctx, vera); err != nil {
		t.Fatalf("second EnsureUser: %v", err)
	}
	mine, err := s.Plan(ctx, vera.ID)
	if err != nil {
		t.Fatal(err)
	}
	liza, _ := s.Plan(ctx, "u-liza")
	if len(mine) == 0 || len(mine) != len(liza) {
		t.Fatalf("hired intern sees %d items, an intern of the same team %d", len(mine), len(liza))
	}
	for _, e := range mine {
		if e.Status != domain.ItemTodo {
			t.Errorf("item %s is %v for a new intern, want todo", e.Item.Key, e.Status)
		}
	}
	if _, err := s.CreateThread(ctx, vera.ID, domain.ThreadGeneral, "", domain.ThreadPin{}); err != nil {
		t.Errorf("thread of the hired intern: %v", err)
	}
}
