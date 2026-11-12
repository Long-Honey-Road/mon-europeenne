package plan

import (
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func entry(key string, period domain.PlanPeriod, st domain.PlanItemStatus, deps ...string) domain.PlanEntry {
	return domain.PlanEntry{Item: domain.PlanItem{Key: key, Period: period, DependsOn: deps}, Status: st}
}

func keys(items []Item) []string {
	out := []string{}
	for _, it := range items {
		out = append(out, it.Item.Key)
	}
	return out
}

func TestBuildCountsAndBlockedBy(t *testing.T) {
	v := Build([]domain.PlanEntry{
		entry("a", domain.PeriodWeek1, domain.ItemDone),
		entry("b", domain.PeriodWeek1, domain.ItemTodo),
		entry("c", domain.PeriodWeek1, domain.ItemTodo, "a", "b"),
	})
	if v.Done != 1 || v.Total != 3 {
		t.Fatalf("done/total = %d/%d", v.Done, v.Total)
	}
	if got := v.Items[2].BlockedBy; len(got) != 1 || got[0] != "b" {
		t.Fatalf("blocked_by = %v, want [b]", got)
	}
	if v.Items[0].BlockedBy == nil || v.Items[0].Waiting() {
		t.Fatal("an item without open dependencies has an empty, non-nil blocked_by")
	}
}

func TestTodayPicksDoingFirstAndSkipsWaiting(t *testing.T) {
	start := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 13, 22, 0, 0, 0, time.UTC)
	today := TodayOf([]domain.PlanEntry{
		entry("done", domain.PeriodWeek1, domain.ItemDone),
		entry("todo1", domain.PeriodWeek1, domain.ItemTodo),
		entry("waits", domain.PeriodWeek1, domain.ItemTodo, "todo1"),
		entry("stuck", domain.PeriodWeek1, domain.ItemBlocked),
		entry("todo2", domain.PeriodWeek2, domain.ItemTodo),
		entry("todo3", domain.PeriodWeek2, domain.ItemTodo),
		entry("doing", domain.PeriodWeek2, domain.ItemDoing),
	}, start, now)

	if today.Day != 5 || today.Week != 1 {
		t.Fatalf("day/week = %d/%d, want 5/1", today.Day, today.Week)
	}
	if today.Period != domain.PeriodWeek1 {
		t.Fatalf("period = %v", today.Period)
	}
	if got := keys(today.Next); len(got) != 3 || got[0] != "doing" || got[1] != "todo1" || got[2] != "todo2" {
		t.Fatalf("next = %v", got)
	}
	if got := keys(today.Blocked); len(got) != 1 || got[0] != "stuck" {
		t.Fatalf("blocked = %v", got)
	}
	if today.Done != 1 || today.Total != 7 {
		t.Fatalf("done/total = %d/%d", today.Done, today.Total)
	}
}

func TestTodayEdges(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	if d := TodayOf(nil, time.Time{}, now); d.Day != 0 || d.Period != 0 || len(d.Next) != 0 {
		t.Fatalf("empty plan without start: %+v", d)
	}
	if d := TodayOf(nil, now.AddDate(0, 0, 3), now); d.Day != 0 {
		t.Fatalf("before the start day = %d", d.Day)
	}
	if d := TodayOf(nil, now.AddDate(0, 0, -7), now); d.Day != 8 || d.Week != 2 {
		t.Fatalf("week two: %d/%d", d.Day, d.Week)
	}
	all := TodayOf([]domain.PlanEntry{
		entry("a", domain.PeriodWeek1, domain.ItemDone),
		entry("z", domain.PeriodFinal, domain.ItemDone),
	}, time.Time{}, now)
	if all.Period != domain.PeriodFinal || len(all.Next) != 0 {
		t.Fatalf("all done: %+v", all)
	}
}
