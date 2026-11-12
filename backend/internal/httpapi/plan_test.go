package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

// memPlans holds one shared plan; statuses are per user.
type memPlans struct {
	items  []domain.PlanItem
	status map[string]map[string]domain.PlanEntry
	err    error
}

func newMemPlans() *memPlans {
	return &memPlans{
		items: []domain.PlanItem{
			{Key: "w1-access", Title: "Запросить доступы", Period: domain.PeriodWeek1, Map: domain.MapPosition{X: 0, Y: 1, Group: "access"}},
			{Key: "w1-ssh", Title: "SSH-ключ", Period: domain.PeriodWeek1, DependsOn: []string{"w1-access"}, Direction: domain.DirectionBackend},
			{Key: "w1-team", Title: "Команда", Period: domain.PeriodWeek1, TeamID: "t-messenger"},
			{Key: "w2-docs", Title: "Документы", Period: domain.PeriodWeek2, Sources: []string{"doc-readme"}},
		},
		status: map[string]map[string]domain.PlanEntry{},
	}
}

func (m *memPlans) Plan(_ context.Context, userID string) ([]domain.PlanEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	var out []domain.PlanEntry
	for _, it := range m.items {
		e, ok := m.status[userID][it.Key]
		if !ok {
			e = domain.PlanEntry{Status: domain.ItemTodo}
		}
		e.Item = it
		out = append(out, e)
	}
	return out, nil
}

func (m *memPlans) SetItemStatus(_ context.Context, userID, key string, st domain.PlanItemStatus, note string) error {
	if m.err != nil {
		return m.err
	}
	for _, it := range m.items {
		if it.Key == key {
			if m.status[userID] == nil {
				m.status[userID] = map[string]domain.PlanEntry{}
			}
			m.status[userID][key] = domain.PlanEntry{Status: st, Note: note}
			return nil
		}
	}
	return store.ErrNotFound
}

func planServer(t *testing.T) (*memPlans, func(method, path, user, body string) (int, map[string]any)) {
	t.Helper()
	plans := newMemPlans()
	d := deps(t)
	d.Plans = plans
	d.Now = func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }
	s := newServerWith(t, d)
	tokens := map[string]string{}
	return plans, func(method, path, user, body string) (int, map[string]any) {
		if _, ok := tokens[user]; !ok {
			tokens[user] = login(t, s, user, user+"-demo")
		}
		return call(t, s, method, path, tokens[user], body)
	}
}

func TestGetPlan(t *testing.T) {
	_, do := planServer(t)

	code, body := do(http.MethodGet, "/v1/private/plan", "intern", "")
	if code != http.StatusOK {
		t.Fatalf("code %d %v", code, body)
	}
	items := body["items"].([]any)
	if len(items) != 4 || body["total"] != 4.0 || body["done"] != 0.0 {
		t.Fatalf("body = %v", body)
	}
	first, ssh := items[0].(map[string]any), items[1].(map[string]any)
	if first["id"] != "w1-access" || first["period"] != "PLAN_PERIOD_W1" || first["status"] != "PLAN_ITEM_STATUS_TODO" {
		t.Fatalf("first = %v", first)
	}
	if m := first["map"].(map[string]any); m["y"] != 1.0 || m["group"] != "access" {
		t.Fatalf("map = %v", m)
	}
	if bb := ssh["blocked_by"].([]any); len(bb) != 1 || bb[0] != "w1-access" || ssh["direction"] != "DIRECTION_BACKEND" {
		t.Fatalf("ssh = %v", ssh)
	}
	if deps := first["depends_on"].([]any); len(deps) != 0 {
		t.Fatalf("empty lists must be [] not null: %v", first)
	}
}

func TestUpdatePlanItem(t *testing.T) {
	plans, do := planServer(t)

	code, body := do(http.MethodPatch, "/v1/private/plan/items/w1-access", "intern", `{"status":"PLAN_ITEM_STATUS_DONE","note":"всё выдали"}`)
	if code != http.StatusOK {
		t.Fatalf("code %d %v", code, body)
	}
	item := body["item"].(map[string]any)
	if item["status"] != "PLAN_ITEM_STATUS_DONE" || item["note"] != "всё выдали" {
		t.Fatalf("item = %v", item)
	}
	if plans.status["u-liza"]["w1-access"].Status != domain.ItemDone {
		t.Fatal("status not stored for the caller")
	}
	_, body = do(http.MethodGet, "/v1/private/plan", "intern", "")
	if ssh := body["items"].([]any)[1].(map[string]any); len(ssh["blocked_by"].([]any)) != 0 {
		t.Fatalf("dependency done, still blocked: %v", ssh)
	}

	for name, tc := range map[string]struct {
		path, body string
		want       int
	}{
		"unknown item":   {"/v1/private/plan/items/nope", `{"status":"PLAN_ITEM_STATUS_DONE"}`, http.StatusNotFound},
		"no status":      {"/v1/private/plan/items/w1-access", `{}`, http.StatusBadRequest},
		"unknown status": {"/v1/private/plan/items/w1-access", `{"status":9}`, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			if code, body := do(http.MethodPatch, tc.path, "intern", tc.body); code != tc.want {
				t.Fatalf("code %d, want %d: %v", code, tc.want, body)
			}
		})
	}
}

func TestPlanStorageDown(t *testing.T) {
	plans, do := planServer(t)
	plans.err = errors.New("db down")
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/private/plan", ""},
		{http.MethodGet, "/v1/private/today", ""},
		{http.MethodPatch, "/v1/private/plan/items/w1-access", `{"status":"PLAN_ITEM_STATUS_DONE"}`},
	} {
		if code, _ := do(tc.method, tc.path, "intern", tc.body); code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: code %d, want 503", tc.method, tc.path, code)
		}
	}
}

func TestToday(t *testing.T) {
	plans, do := planServer(t)
	plans.status["u-liza"] = map[string]domain.PlanEntry{
		"w1-team": {Status: domain.ItemBlocked, Note: "наставник в отпуске"},
		"w2-docs": {Status: domain.ItemDoing},
	}

	code, body := do(http.MethodGet, "/v1/private/today", "intern", "")
	if code != http.StatusOK {
		t.Fatalf("code %d %v", code, body)
	}
	// Liza started on 2026-11-09; the clock says 2026-11-13.
	if body["day"] != 5.0 || body["week"] != 1.0 || body["period"] != "PLAN_PERIOD_W1" {
		t.Fatalf("body = %v", body)
	}
	next := body["next_steps"].([]any)
	if len(next) != 2 || next[0].(map[string]any)["id"] != "w2-docs" || next[1].(map[string]any)["id"] != "w1-access" {
		t.Fatalf("next_steps = %v", next)
	}
	blockers := body["blockers"].([]any)
	if len(blockers) != 1 {
		t.Fatalf("blockers = %v", blockers)
	}
	if b := blockers[0].(map[string]any); b["kind"] != "plan_item" || b["id"] != "w1-team" || b["reason"] != "наставник в отпуске" {
		t.Fatalf("blocker = %v", b)
	}
}
