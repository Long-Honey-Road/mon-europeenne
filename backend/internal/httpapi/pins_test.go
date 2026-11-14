package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/stream"
)

type memPins struct {
	mu   sync.Mutex
	msgs map[string]domain.Message
	pins []domain.Pin
}

func (m *memPins) CreatePin(_ context.Context, p domain.Pin) (domain.Pin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p.ID, p.CreatedAt, p.UpdatedAt = fmt.Sprint("pin-", len(m.pins)+1), time.Now(), time.Now()
	m.pins = append(m.pins, p)
	return p, nil
}

func (m *memPins) Pins(_ context.Context, userID string, f domain.Focus) ([]domain.Pin, error) {
	var out []domain.Pin
	for _, p := range m.pins {
		if p.UserID == userID && (f.IsZero() || p.Focus == f) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *memPins) UpdatePin(_ context.Context, userID, id, title, text string) (domain.Pin, error) {
	for i, p := range m.pins {
		if p.ID == id && p.UserID == userID {
			m.pins[i].Title, m.pins[i].Text = title, text
			return m.pins[i], nil
		}
	}
	return domain.Pin{}, store.ErrNotFound
}

func (m *memPins) DeletePin(_ context.Context, userID, id string) error {
	for i, p := range m.pins {
		if p.ID == id && p.UserID == userID {
			m.pins = append(m.pins[:i], m.pins[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *memPins) OwnMessage(_ context.Context, userID, id string) (domain.Message, error) {
	msg, ok := m.msgs[id]
	if !ok || !strings.HasPrefix(msg.ThreadID, userID) {
		return domain.Message{}, store.ErrNotFound
	}
	return msg, nil
}

type pubRecorder struct{ events []stream.Event }

func (p *pubRecorder) Publish(_ context.Context, _ string, e stream.Event) error {
	p.events = append(p.events, e)
	return nil
}

// Spec 008: the pin button keeps an answer on a sphere; only the owner sees and changes it.
func TestPinsAPI(t *testing.T) {
	access := domain.Focus{Kind: domain.FocusPlanItem, ID: "w1-access"}
	pins := &memPins{msgs: map[string]domain.Message{
		"m1": {ID: "m1", ThreadID: "u-liza/plan", Role: domain.MessageAssistant, Focus: access,
			Content: "**Порядок доступов**\nСначала Jira [1], потом VPN [2].",
			Blocks:  json.RawMessage(`[{"type":"sources","items":[{"ref":1,"source_id":"c1"}]}]`)},
		"m2": {ID: "m2", ThreadID: "u-liza/general", Role: domain.MessageAssistant, Content: "Без сферы"},
	}}
	pub := &pubRecorder{}
	d := deps(t)
	d.Pins, d.Publish, d.Plans = pins, pub, newMemPlans()
	s := newServerWith(t, d)
	liza, anna := login(t, s, "intern", "intern-demo"), login(t, s, "mentor", "mentor-demo")

	code, body := call(t, s, http.MethodPost, "/v1/private/messages/m1/pin", liza, `{}`)
	if code != http.StatusOK {
		t.Fatalf("pin: %d %v", code, body)
	}
	pin := body["pin"].(map[string]any)
	if pin["title"] != "Порядок доступов" || pin["text"] != "**Порядок доступов**\nСначала Jira, потом VPN." || pin["message_id"] != "m1" ||
		pin["focus"].(map[string]any)["id"] != "w1-access" {
		t.Errorf("pin = %v", pin)
	}
	if string(pins.pins[0].Sources) != `[{"ref":1,"source_id":"c1"}]` {
		t.Errorf("sources = %s", pins.pins[0].Sources)
	}
	if len(pub.events) != 1 || pub.events[0].Type != "pinned" {
		t.Errorf("published %+v", pub.events)
	}

	for name, c := range map[string]struct {
		token, path, body string
		want              int
	}{
		"message without a sphere": {liza, "/v1/private/messages/m2/pin", `{}`, http.StatusBadRequest},
		"given sphere":             {liza, "/v1/private/messages/m2/pin", `{"focus":{"kind":"FOCUS_KIND_PLAN_ITEM","id":"w1-ssh"},"title":"SSH"}`, http.StatusOK},
		"sphere not in the plan":   {liza, "/v1/private/messages/m2/pin", `{"focus":{"kind":"FOCUS_KIND_PLAN_ITEM","id":"ghost"}}`, http.StatusNotFound},
		"someone else's message":   {anna, "/v1/private/messages/m1/pin", `{}`, http.StatusNotFound},
	} {
		if code, b := call(t, s, http.MethodPost, c.path, c.token, c.body); code != c.want {
			t.Errorf("%s: %d %v, want %d", name, code, b, c.want)
		}
	}

	_, list := call(t, s, http.MethodGet, "/v1/private/pins?kind=FOCUS_KIND_PLAN_ITEM&id=w1-access", liza, "")
	if n := len(list["pins"].([]any)); n != 1 {
		t.Errorf("sphere pins = %d", n)
	}
	if _, all := call(t, s, http.MethodGet, "/v1/private/pins", anna, ""); len(all["pins"].([]any)) != 0 {
		t.Errorf("mentor sees the intern's pins: %v", all)
	}

	if code, b := call(t, s, http.MethodPatch, "/v1/private/pins/pin-1", liza, `{"title":"Доступы","text":"Jira, потом VPN"}`); code != http.StatusOK || b["pin"].(map[string]any)["title"] != "Доступы" {
		t.Errorf("update: %d %v", code, b)
	}
	if code, _ := call(t, s, http.MethodPatch, "/v1/private/pins/pin-1", liza, `{"title":"","text":"x"}`); code != http.StatusBadRequest {
		t.Errorf("empty title: %d", code)
	}
	if code, _ := call(t, s, http.MethodPatch, "/v1/private/pins/pin-1", anna, `{"title":"x","text":"y"}`); code != http.StatusNotFound {
		t.Errorf("someone else's update: %d", code)
	}
	if code, _ := call(t, s, http.MethodDelete, "/v1/private/pins/pin-1", anna, ""); code != http.StatusNotFound {
		t.Errorf("someone else's delete: %d", code)
	}
	if code, _ := call(t, s, http.MethodDelete, "/v1/private/pins/pin-1", liza, ""); code != http.StatusOK {
		t.Errorf("delete: %d", code)
	}
}

func TestPinsAPIWithoutStoreIsUnimplemented(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "intern", "intern-demo")
	if code, _ := call(t, s, http.MethodGet, "/v1/private/pins", token, ""); code != http.StatusNotImplemented {
		t.Errorf("pins without a store: %d, want 501", code)
	}
}
