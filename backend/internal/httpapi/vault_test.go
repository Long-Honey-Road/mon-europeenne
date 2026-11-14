package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

type memVault struct {
	items map[string]domain.VaultItem
}

func (m *memVault) CreateVaultItem(_ context.Context, v domain.VaultItem) (domain.VaultItem, error) {
	v.ID, v.Version, v.UpdatedAt = fmt.Sprint("v", len(m.items)+1), 1, time.Now()
	v.Versions = nil
	m.items[v.ID] = v
	return v, nil
}

func (m *memVault) VaultItems(_ context.Context, userID, tag string) ([]domain.VaultItem, error) {
	var out []domain.VaultItem
	for _, v := range m.items {
		if v.UserID == userID && (tag == "" || strings.Contains(strings.Join(v.Tags, ","), tag)) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (m *memVault) VaultItem(_ context.Context, userID, id string) (domain.VaultItem, error) {
	v, ok := m.items[id]
	if !ok || v.UserID != userID {
		return domain.VaultItem{}, store.ErrNotFound
	}
	v.Versions = []domain.VaultItem{{Version: v.Version, Title: v.Title, Summary: v.Summary, CreatedAt: time.Now()}}
	return v, nil
}

func (m *memVault) UpdateVaultItem(ctx context.Context, userID, id string, p domain.VaultPatch) (domain.VaultItem, error) {
	v, err := m.VaultItem(ctx, userID, id)
	if err != nil {
		return v, err
	}
	if p.Summary != nil {
		v.Summary = *p.Summary
	}
	if p.Title != nil {
		v.Title = *p.Title
	}
	if p.Tags != nil {
		v.Tags = p.Tags
	}
	v.Version++
	v.Versions = nil
	m.items[id] = v
	return v, nil
}

func (m *memVault) RestoreVaultVersion(ctx context.Context, userID, id string, version int) (domain.VaultItem, error) {
	v, err := m.VaultItem(ctx, userID, id)
	if err != nil || version != 1 {
		return domain.VaultItem{}, store.ErrNotFound
	}
	v.Version++
	v.Versions = nil
	return v, nil
}

func (m *memVault) DeleteVaultItem(_ context.Context, userID, id string) error {
	if v, ok := m.items[id]; !ok || v.UserID != userID {
		return store.ErrNotFound
	}
	delete(m.items, id)
	return nil
}

// Spec 009: the vault is private, versioned and fed by the button under an answer.
func TestVaultAPI(t *testing.T) {
	vault := &memVault{items: map[string]domain.VaultItem{}}
	pins := &memPins{msgs: map[string]domain.Message{
		"m1": {ID: "m1", ThreadID: "u-liza/general", Role: domain.MessageAssistant, Content: "## VPN\nЗаявка на VPN [1], согласует руководитель."},
	}}
	st, jb := newMemStore(), &memJobs{}
	d := deps(t)
	d.Vault, d.Pins, d.Threads, d.Jobs, d.Plans = vault, pins, st, jb, newMemPlans()
	s := newServerWith(t, d)
	liza, anna := login(t, s, "intern", "intern-demo"), login(t, s, "mentor", "mentor-demo")

	code, body := call(t, s, http.MethodPost, "/v1/private/messages/m1/vault", liza, `{}`)
	if code != http.StatusOK {
		t.Fatalf("save: %d %v", code, body)
	}
	item := body["item"].(map[string]any)
	if item["title"] != "VPN" || item["summary"] != "## VPN\nЗаявка на VPN, согласует руководитель." || item["version"] != float64(1) {
		t.Errorf("item = %v", item)
	}
	if src := item["sources"].([]any)[0].(map[string]any); src["kind"] != "message" || src["id"] != "m1" {
		t.Errorf("sources = %v", item["sources"])
	}
	id := item["id"].(string)

	if _, b := call(t, s, http.MethodGet, "/v1/private/vault/"+id, liza, ""); len(b["item"].(map[string]any)["versions"].([]any)) != 1 {
		t.Errorf("get = %v", b)
	}
	if code, b := call(t, s, http.MethodPatch, "/v1/private/vault/"+id, liza, `{"summary":"Короче","tags":["vpn"]}`); code != http.StatusOK || b["item"].(map[string]any)["version"] != float64(2) {
		t.Errorf("update: %d %v", code, b)
	}
	if code, b := call(t, s, http.MethodPost, "/v1/private/vault/"+id+"/restore", liza, `{"version":1}`); code != http.StatusOK || b["item"].(map[string]any)["version"] != float64(3) {
		t.Errorf("restore: %d %v", code, b)
	}
	if _, b := call(t, s, http.MethodGet, "/v1/private/vault?tag=vpn", liza, ""); len(b["items"].([]any)) != 1 {
		t.Errorf("list by tag = %v", b)
	}

	for name, c := range map[string]struct {
		method, path, token, body string
		want                      int
	}{
		"mentor reads the intern's vault": {http.MethodGet, "/v1/private/vault/" + id, anna, "", http.StatusNotFound},
		"mentor lists":                    {http.MethodGet, "/v1/private/vault", anna, "", http.StatusOK},
		"mentor saves her message":        {http.MethodPost, "/v1/private/messages/m1/vault", anna, `{}`, http.StatusNotFound},
		"empty patch":                     {http.MethodPatch, "/v1/private/vault/" + id, liza, `{}`, http.StatusBadRequest},
		"long summary":                    {http.MethodPatch, "/v1/private/vault/" + id, liza, `{"summary":"` + strings.Repeat("я", 1300) + `"}`, http.StatusBadRequest},
		"long title":                      {http.MethodPatch, "/v1/private/vault/" + id, liza, `{"title":"` + strings.Repeat("я", 90) + `"}`, http.StatusBadRequest},
		"restore a missing version":       {http.MethodPost, "/v1/private/vault/" + id + "/restore", liza, `{"version":7}`, http.StatusNotFound},
		"mentor deletes":                  {http.MethodDelete, "/v1/private/vault/" + id, anna, "", http.StatusNotFound},
	} {
		code, b := call(t, s, c.method, c.path, c.token, c.body)
		if code != c.want {
			t.Errorf("%s: %d %v, want %d", name, code, b, c.want)
		}
		if name == "mentor lists" && len(b["items"].([]any)) != 0 {
			t.Errorf("mentor sees %v", b["items"])
		}
	}

	// A focus on a vault record is checked like a plan item.
	_, th := call(t, s, http.MethodPost, "/v1/private/threads", liza, `{"type":"THREAD_TYPE_VAULT"}`)
	vt := th["thread"].(map[string]any)["id"].(string)
	if _, again := call(t, s, http.MethodPost, "/v1/private/threads", liza, `{"type":"THREAD_TYPE_VAULT"}`); again["thread"].(map[string]any)["id"] != vt {
		t.Error("a second vault thread was created")
	}
	if code, _ := call(t, s, http.MethodPost, "/v1/private/threads/"+vt+"/messages", liza, `{"content":"сократи","focus":{"kind":"FOCUS_KIND_VAULT_ITEM","id":"`+id+`"}}`); code != http.StatusAccepted {
		t.Errorf("focus on own record: %d", code)
	}
	_ = jb.ReleaseChatTurn(t.Context(), vt, "job-1")
	if code, _ := call(t, s, http.MethodPost, "/v1/private/threads/"+vt+"/messages", liza, `{"content":"сократи","focus":{"kind":"FOCUS_KIND_VAULT_ITEM","id":"v99"}}`); code != http.StatusNotFound {
		t.Errorf("focus on a missing record: %d", code)
	}

	if code, _ := call(t, s, http.MethodDelete, "/v1/private/vault/"+id, liza, ""); code != http.StatusOK {
		t.Errorf("delete: %d", code)
	}
	raw, _ := json.Marshal(vault.items)
	if string(raw) != "{}" {
		t.Errorf("left %s", raw)
	}
}

func TestVaultAPIWithoutStoreIsUnimplemented(t *testing.T) {
	s := newServer(t)
	token := login(t, s, "intern", "intern-demo")
	if code, _ := call(t, s, http.MethodGet, "/v1/private/vault", token, ""); code != http.StatusNotImplemented {
		t.Errorf("vault without a store: %d, want 501", code)
	}
}
