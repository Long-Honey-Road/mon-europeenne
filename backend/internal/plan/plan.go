// Package plan derives what screens and tools show from a user's plan: blockers, progress and next steps.
package plan

import (
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// MaxNextSteps is how many next steps "today" offers.
const MaxNextSteps = 3

// Item is a plan entry with the dependencies that are not done yet.
type Item struct {
	domain.PlanEntry
	BlockedBy []string
}

// Waiting is true when the item cannot start because a dependency is not done.
func (i Item) Waiting() bool { return len(i.BlockedBy) > 0 }

type View struct {
	Items []Item
	Done  int
	Total int
}

// Build keeps the store's order (period, then map position).
func Build(entries []domain.PlanEntry) View {
	done := make(map[string]bool, len(entries))
	for _, e := range entries {
		done[e.Item.Key] = e.Status == domain.ItemDone
	}
	v := View{Items: make([]Item, 0, len(entries)), Total: len(entries)}
	for _, e := range entries {
		if e.Status == domain.ItemDone {
			v.Done++
		}
		it := Item{PlanEntry: e, BlockedBy: []string{}}
		for _, dep := range e.Item.DependsOn {
			if !done[dep] {
				it.BlockedBy = append(it.BlockedBy, dep)
			}
		}
		v.Items = append(v.Items, it)
	}
	return v
}

type Today struct {
	// Day is the calendar day since the start date, 1-based; 0 before the start or without a start date.
	Day  int
	Week int
	// Period is the earliest period with unfinished items, the last one when everything is done.
	Period  domain.PlanPeriod
	Next    []Item
	Blocked []Item
	Done    int
	Total   int
}

// TodayOf picks the next steps (items in progress first) and the items marked blocked. The day is counted
// by calendar dates of start and of now in now's location.
func TodayOf(entries []domain.PlanEntry, start, now time.Time) Today {
	v := Build(entries)
	t := Today{Done: v.Done, Total: v.Total, Next: []Item{}, Blocked: []Item{}}
	if !start.IsZero() {
		y, m, d := start.Date()
		sy, sm, sd := now.Date()
		days := int(time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC).Sub(time.Date(y, m, d, 0, 0, 0, 0, time.UTC)).Hours() / 24)
		if days >= 0 {
			t.Day, t.Week = days+1, days/7+1
		}
	}

	var doing, ready []Item
	for _, it := range v.Items {
		switch {
		case it.Status == domain.ItemDone:
		case it.Status == domain.ItemBlocked:
			t.Blocked = append(t.Blocked, it)
		case it.Waiting():
		case it.Status == domain.ItemDoing:
			doing = append(doing, it)
		default:
			ready = append(ready, it)
		}
		if it.Status != domain.ItemDone && (t.Period == 0 || it.Item.Period < t.Period) {
			t.Period = it.Item.Period
		}
	}
	if t.Period == 0 && len(v.Items) > 0 {
		t.Period = v.Items[len(v.Items)-1].Item.Period
	}
	for _, it := range append(doing, ready...) {
		if len(t.Next) == MaxNextSteps {
			break
		}
		t.Next = append(t.Next, it)
	}
	return t
}
