package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

type planOf map[string][]domain.PlanEntry

func (p planOf) Plan(_ context.Context, userID string) ([]domain.PlanEntry, error) {
	if userID == "u-broken" {
		return nil, errors.New("db down")
	}
	return p[userID], nil
}

func (planOf) SetItemStatus(context.Context, string, string, domain.PlanItemStatus, string) error {
	return nil
}

type pinRecorder struct {
	pins []domain.Pin
	err  error
}

func (r *pinRecorder) CreatePin(_ context.Context, p domain.Pin) (domain.Pin, error) {
	p.ID = "pin-1"
	r.pins = append(r.pins, p)
	return p, r.err
}

var lizaPlan = planOf{"u-liza": {
	{Item: domain.PlanItem{Key: "w1-access", Title: "Запросить доступы"}},
	{Item: domain.PlanItem{Key: "w1-ssh-keys", Title: "SSH-ключ"}},
}}

func sphereRegistry(t *testing.T, pins PinStore) *Registry {
	t.Helper()
	pin, err := PinNote(lizaPlan, pins)
	if err != nil {
		t.Fatal(err)
	}
	hl, _ := HighlightSpheres(lizaPlan)
	fs, _ := FocusSphere(lizaPlan)
	reg, err := NewRegistry(pin, hl, fs)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func callAs(t *testing.T, reg *Registry, user string, thread domain.ThreadType, name, args string) (map[string]any, error) {
	t.Helper()
	raw, err := reg.Call(context.Background(), domain.Principal{UserID: user, Role: domain.RoleIntern}, thread, name, json.RawMessage(args))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	return out, json.Unmarshal(raw, &out)
}

// Golden dialog 3 of spec 008.
func TestPinNote(t *testing.T) {
	rec := &pinRecorder{}
	reg := sphereRegistry(t, rec)
	out, err := callAs(t, reg, "u-liza", domain.ThreadPlan, "pin_note",
		`{"id":"w1-access","title":"`+strings.Repeat("Д", 70)+`","text":"Сначала Jira и VPN"}`)
	if err != nil {
		t.Fatal(err)
	}
	ev := out["ui_event"].(map[string]any)
	if ev["type"] != "pinned" || ev["data"].(map[string]any)["id"] != "w1-access" || out["pin_id"] != "pin-1" {
		t.Errorf("result = %v", out)
	}
	if p := rec.pins[0]; p.UserID != "u-liza" || p.Focus.ID != "w1-access" || len([]rune(p.Title)) != maxPinTitle {
		t.Errorf("pin = %+v", p)
	}
	for name, c := range map[string]struct {
		user, args string
		want       error
	}{
		"not in the plan": {"u-liza", `{"id":"m9-ghost","title":"x","text":"y"}`, ErrNotFound},
		"empty text":      {"u-liza", `{"id":"w1-access","title":"x","text":" "}`, ErrInvalidArgs},
		"too long":        {"u-liza", `{"id":"w1-access","title":"x","text":"` + strings.Repeat("я", 700) + `"}`, ErrInvalidArgs},
		"other kind":      {"u-liza", `{"kind":"vault_item","id":"v1","title":"x","text":"y"}`, ErrInvalidArgs},
		"someone else":    {"u-anna", `{"id":"w1-access","title":"x","text":"y"}`, ErrNotFound},
	} {
		if _, err := callAs(t, reg, c.user, domain.ThreadPlan, "pin_note", c.args); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if _, err := callAs(t, reg, "u-liza", domain.ThreadGeneral, "pin_note", `{"id":"w1-access","title":"x","text":"y"}`); !errors.Is(err, ErrNotInThread) {
		t.Errorf("general thread: %v", err)
	}
	if _, err := callAs(t, reg, "u-broken", domain.ThreadPlan, "pin_note", `{"id":"w1-access","title":"x","text":"y"}`); err == nil {
		t.Error("plan error swallowed")
	}
	if _, err := callAs(t, sphereRegistry(t, &pinRecorder{err: errors.New("down")}), "u-liza", domain.ThreadPlan, "pin_note", `{"id":"w1-access","title":"x","text":"y"}`); err == nil {
		t.Error("store error swallowed")
	}
}

// Golden dialog 5 of spec 008.
func TestHighlightAndFocusSphere(t *testing.T) {
	reg := sphereRegistry(t, &pinRecorder{})
	out, err := callAs(t, reg, "u-liza", domain.ThreadGeneral, "highlight_spheres", `{"ids":["w1-access","ghost","w1-ssh-keys"],"note":"ждут доступов"}`)
	if err != nil {
		t.Fatal(err)
	}
	data := out["ui_event"].(map[string]any)["data"].(map[string]any)
	if ids := data["ids"].([]any); len(ids) != 2 || data["kind"] != "plan_item" || data["note"] != "ждут доступов" {
		t.Errorf("highlight = %v", data)
	}
	if _, err := callAs(t, reg, "u-liza", domain.ThreadGeneral, "highlight_spheres", `{"ids":["ghost"]}`); !errors.Is(err, ErrNotFound) {
		t.Errorf("only unknown ids: %v", err)
	}
	fo, err := callAs(t, reg, "u-liza", domain.ThreadTask, "focus_sphere", `{"id":"w1-ssh-keys","reason":"там про ключ"}`)
	if err != nil {
		t.Fatal(err)
	}
	if d := fo["ui_event"].(map[string]any)["data"].(map[string]any); d["title"] != "SSH-ключ" || fo["ui_event"].(map[string]any)["type"] != "focus_suggest" {
		t.Errorf("focus = %v", fo)
	}
	for _, c := range []struct{ name, args string }{
		{"highlight_spheres", `{"kind":"vault_item","ids":["x"]}`}, {"focus_sphere", `{"kind":"vault_item","id":"x"}`},
		{"focus_sphere", `{"id":"ghost"}`},
	} {
		if _, err := callAs(t, reg, "u-liza", domain.ThreadGeneral, c.name, c.args); err == nil {
			t.Errorf("%s %s accepted", c.name, c.args)
		}
	}
	for _, name := range []string{"highlight_spheres", "focus_sphere"} {
		if _, err := callAs(t, reg, "u-broken", domain.ThreadGeneral, name, `{"id":"x","ids":["x"]}`); err == nil {
			t.Errorf("%s: plan error swallowed", name)
		}
	}
}
