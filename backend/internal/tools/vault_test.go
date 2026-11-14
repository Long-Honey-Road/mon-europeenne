package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

type memVault struct {
	items map[string]domain.VaultItem
	err   error
}

func (m *memVault) CreateVaultItem(_ context.Context, v domain.VaultItem) (domain.VaultItem, error) {
	v.ID, v.Version = "v1", 1
	m.items[v.ID] = v
	return v, m.err
}

func (m *memVault) VaultItems(_ context.Context, userID, tag string) ([]domain.VaultItem, error) {
	var out []domain.VaultItem
	for _, v := range m.items {
		if v.UserID == userID && (tag == "" || strings.Contains(strings.Join(v.Tags, ","), tag)) {
			out = append(out, v)
		}
	}
	return out, m.err
}

func (m *memVault) VaultItem(_ context.Context, userID, id string) (domain.VaultItem, error) {
	v, ok := m.items[id]
	if !ok || v.UserID != userID {
		return domain.VaultItem{}, ErrNotFound
	}
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
	m.items[id] = v
	return v, m.err
}

func vaultRegistry(t *testing.T, v VaultStore) *Registry {
	t.Helper()
	save, err := VaultSave(v)
	if err != nil {
		t.Fatal(err)
	}
	update, _ := VaultUpdate(v)
	list, _ := VaultList(v)
	reg, err := NewRegistry(save, update, list)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// Golden dialogs 1–4 of spec 009.
func TestVaultTools(t *testing.T) {
	v := &memVault{items: map[string]domain.VaultItem{}}
	reg := vaultRegistry(t, v)
	out, err := callAs(t, reg, "u-liza", domain.ThreadGeneral, "vault_save",
		`{"title":"Доступы","summary":"Семь доступов, Jira первой","tags":[" Доступы ",""],"sources":[{"kind":"message","id":"m1"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if out["item_id"] != "v1" || out["ui_event"].(map[string]any)["type"] != "vault_saved" {
		t.Errorf("save = %v", out)
	}
	if got := v.items["v1"]; got.UserID != "u-liza" || len(got.Tags) != 1 || got.Tags[0] != "доступы" || !json.Valid(got.Sources) {
		t.Errorf("saved = %+v", got)
	}
	for name, args := range map[string]string{
		"too long":   `{"title":"x","summary":"` + strings.Repeat("я", 1500) + `","sources":[{"kind":"message","id":"m1"}]}`,
		"no sources": `{"title":"x","summary":"y","sources":[]}`,
		"no title":   `{"title":" ","summary":"y","sources":[{"kind":"message","id":"m1"}]}`,
		"no summary": `{"title":"x","summary":" ","sources":[{"kind":"message","id":"m1"}]}`,
	} {
		if _, err := callAs(t, reg, "u-liza", domain.ThreadGeneral, "vault_save", args); !errors.Is(err, ErrInvalidArgs) {
			t.Errorf("%s: %v", name, err)
		}
	}

	up, err := callAs(t, reg, "u-liza", domain.ThreadVault, "vault_update", `{"item_id":"v1","summary":"Jira, VPN, Sentry","tags":["w1"]}`)
	if err != nil || up["version"] != float64(2) {
		t.Errorf("update = %v, %v", up, err)
	}
	for name, c := range map[string]struct{ user, args string }{
		"nothing to change": {"u-liza", `{"item_id":"v1"}`},
		"too long":          {"u-liza", `{"item_id":"v1","summary":"` + strings.Repeat("я", 1300) + `"}`},
		"someone else's":    {"u-anna", `{"item_id":"v1","title":"x"}`},
	} {
		if _, err := callAs(t, reg, c.user, domain.ThreadVault, "vault_update", c.args); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := callAs(t, reg, "u-liza", domain.ThreadGeneral, "vault_update", `{"item_id":"v1","title":"x"}`); !errors.Is(err, ErrNotInThread) {
		t.Errorf("update outside the vault thread: %v", err)
	}
	if _, err := callAs(t, reg, "u-liza", domain.ThreadVault, "vault_update", `{"item_id":"v1","title":"Новое"}`); err != nil {
		t.Errorf("title only: %v", err)
	}

	list, err := callAs(t, reg, "u-liza", domain.ThreadPlan, "vault_list", `{"tag":"W1"}`)
	if err != nil || len(list["items"].([]any)) != 1 {
		t.Errorf("list = %v, %v", list, err)
	}
	if mine, _ := callAs(t, reg, "u-anna", domain.ThreadGeneral, "vault_list", `{}`); len(mine["items"].([]any)) != 0 {
		t.Errorf("another user's list = %v", mine)
	}
	broken := vaultRegistry(t, &memVault{items: map[string]domain.VaultItem{}, err: errors.New("down")})
	for _, c := range []struct{ name, args string }{
		{"vault_save", `{"title":"x","summary":"y","sources":[{"kind":"message","id":"m1"}]}`}, {"vault_list", `{}`},
	} {
		if _, err := callAs(t, broken, "u-liza", domain.ThreadGeneral, c.name, c.args); err == nil {
			t.Errorf("%s: store error swallowed", c.name)
		}
	}
}
