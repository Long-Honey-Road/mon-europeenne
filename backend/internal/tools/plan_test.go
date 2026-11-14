package tools

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// memPlan is a PlanStore over fixed entries; visibility rules are tested on Postgres in internal/store.
type memPlan struct {
	entries []domain.PlanEntry
	failed  error
}

func (m *memPlan) Plan(context.Context, string) ([]domain.PlanEntry, error) {
	return append([]domain.PlanEntry(nil), m.entries...), m.failed
}

func (m *memPlan) SetItemStatus(_ context.Context, _ string, key string, st domain.PlanItemStatus, note string) error {
	for i := range m.entries {
		if m.entries[i].Item.Key == key {
			m.entries[i].Status, m.entries[i].Note = st, note
			return nil
		}
	}
	return domain.ErrNotFound
}

func samplePlan() *memPlan {
	item := func(key string, p domain.PlanPeriod, deps ...string) domain.PlanItem {
		return domain.PlanItem{Key: key, Title: "Пункт " + key, Practice: "Сделай " + key, Period: p, DependsOn: deps}
	}
	return &memPlan{entries: []domain.PlanEntry{
		{Item: item("w1-team", domain.PeriodWeek1), Status: domain.ItemDone},
		{Item: item("w1-access", domain.PeriodWeek1), Status: domain.ItemDoing},
		{Item: item("w1-channels", domain.PeriodWeek1, "w1-access"), Status: domain.ItemTodo},
		{Item: item("w1-notes", domain.PeriodWeek1, "w1-access"), Status: domain.ItemBlocked, Note: "Нужен доступ к Confluence"},
		{Item: item("m1-grafana", domain.PeriodMonth1, "w1-access", "w1-team"), Status: domain.ItemTodo},
	}}
}

func planRegistry(t *testing.T, plan *memPlan) *Registry {
	t.Helper()
	get, err := GetPlan(plan)
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	upd, err := UpdateItemStatus(plan)
	if err != nil {
		t.Fatalf("UpdateItemStatus: %v", err)
	}
	return newRegistry(t, get, upd)
}

type planOut struct {
	Items []struct {
		ItemID    string   `json:"item_id"`
		Status    string   `json:"status"`
		Period    string   `json:"period"`
		Note      string   `json:"note"`
		Practice  string   `json:"practice"`
		BlockedBy []string `json:"blocked_by"`
	} `json:"items"`
	Done  int `json:"done"`
	Total int `json:"total"`
}

func getPlan(t *testing.T, r *Registry, thread domain.ThreadType, args string) planOut {
	t.Helper()
	raw, err := r.Call(context.Background(), intern, thread, "get_plan", json.RawMessage(args))
	if err != nil {
		t.Fatalf("get_plan: %v", err)
	}
	var out planOut
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

func TestGetPlanShowsStatusesAndBlockers(t *testing.T) {
	out := getPlan(t, planRegistry(t, samplePlan()), domain.ThreadPlan, `{}`)

	if len(out.Items) != 5 || out.Done != 1 || out.Total != 5 {
		t.Fatalf("plan = %+v", out)
	}
	channels, grafana, notes := out.Items[2], out.Items[4], out.Items[3]
	if channels.Status != "todo" || !slices.Equal(channels.BlockedBy, []string{"w1-access"}) || channels.Period != "w1" {
		t.Errorf("w1-channels = %+v", channels)
	}
	if !slices.Equal(grafana.BlockedBy, []string{"w1-access"}) {
		t.Errorf("m1-grafana blocked by %v, want only the undone dependency", grafana.BlockedBy)
	}
	if notes.Status != "blocked" || notes.Note != "Нужен доступ к Confluence" || out.Items[0].Practice != "Сделай w1-team" {
		t.Errorf("items = %+v", out.Items)
	}
	if out.Items[0].BlockedBy == nil {
		t.Error("blocked_by must be an empty list, not null")
	}
}

func TestGetPlanFiltersButCountsTheWholePlan(t *testing.T) {
	r := planRegistry(t, samplePlan())

	byPeriod := getPlan(t, r, domain.ThreadGeneral, `{"period":"m1"}`)
	if len(byPeriod.Items) != 1 || byPeriod.Items[0].ItemID != "m1-grafana" || byPeriod.Total != 5 {
		t.Errorf("period filter = %+v", byPeriod)
	}
	byStatus := getPlan(t, r, domain.ThreadTask, `{"status":"todo"}`)
	if len(byStatus.Items) != 2 || byStatus.Done != 1 {
		t.Errorf("status filter = %+v", byStatus)
	}
}

func TestGetPlanRejectsUnknownFilters(t *testing.T) {
	r := planRegistry(t, samplePlan())
	for _, args := range []string{`{"period":"w9"}`, `{"status":"half"}`} {
		if _, err := r.Call(context.Background(), intern, domain.ThreadPlan, "get_plan", json.RawMessage(args)); !errors.Is(err, ErrInvalidArgs) {
			t.Errorf("%s: error = %v, want ErrInvalidArgs", args, err)
		}
	}
}

func TestUpdateItemStatus(t *testing.T) {
	plan := samplePlan()
	r := planRegistry(t, plan)

	raw, err := r.Call(context.Background(), intern, domain.ThreadPlan, "update_item_status",
		json.RawMessage(`{"item_id":"w1-access","status":"done","note":"все семь выданы"}`))
	if err != nil || string(raw) != `{"item_id":"w1-access","status":"done"}` {
		t.Fatalf("update = %s, %v", raw, err)
	}
	out := getPlan(t, r, domain.ThreadPlan, `{}`)
	if out.Done != 2 || len(out.Items[2].BlockedBy) != 0 {
		t.Errorf("after update = %+v", out)
	}
}

func TestUpdateItemStatusRefusals(t *testing.T) {
	r := planRegistry(t, samplePlan())
	cases := map[string]struct {
		thread domain.ThreadType
		args   string
		want   error
	}{
		"unknown item":   {domain.ThreadPlan, `{"item_id":"w9-nothing","status":"done"}`, ErrNotFound},
		"unknown status": {domain.ThreadPlan, `{"item_id":"w1-access","status":"half"}`, ErrInvalidArgs},
		"general thread": {domain.ThreadGeneral, `{"item_id":"w1-access","status":"done"}`, ErrNotInThread},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := r.Call(context.Background(), intern, tc.thread, "update_item_status", json.RawMessage(tc.args)); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestGetPlanReportsStorageFailure(t *testing.T) {
	r := planRegistry(t, &memPlan{failed: errors.New("db down")})
	if _, err := r.Call(context.Background(), intern, domain.ThreadPlan, "get_plan", json.RawMessage(`{}`)); err == nil {
		t.Fatal("storage failure swallowed")
	}
}

// Only interns have an adaptation plan; for others the tools are not offered at all.
func TestPlanToolsAreForInternsOnly(t *testing.T) {
	r := planRegistry(t, samplePlan())
	for _, role := range []domain.Role{domain.RoleMentor, domain.RoleManager, domain.RoleHRBP, domain.RoleHRAdmin} {
		p := domain.Principal{UserID: "u-other", Role: role, TeamID: "t-messenger"}
		for _, tool := range []string{"get_plan", "update_item_status"} {
			if _, err := r.Call(context.Background(), p, domain.ThreadPlan, tool, json.RawMessage(`{"item_id":"w1-access","status":"done"}`)); !errors.Is(err, ErrForbidden) {
				t.Errorf("%s as %v: error = %v, want forbidden", tool, role, err)
			}
		}
	}
}
