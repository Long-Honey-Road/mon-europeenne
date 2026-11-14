package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func TestVaultVersionsAndRestore(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	v, err := s.CreateVaultItem(ctx, domain.VaultItem{UserID: "u-liza", Title: "Доступы", Summary: "Семь доступов",
		Tags: []string{"доступы"}, Sources: json.RawMessage(`[{"kind":"message","id":"m1"}]`)})
	if err != nil || v.Version != 1 || v.ID == "" {
		t.Fatalf("create = %+v, %v", v, err)
	}
	if _, err := s.CreateVaultItem(ctx, domain.VaultItem{UserID: "u-liza", Title: "SSH", Summary: "ed25519"}); err != nil {
		t.Fatal(err)
	}

	short := "Jira, VPN, Sentry"
	up, err := s.UpdateVaultItem(ctx, "u-liza", v.ID, domain.VaultPatch{Summary: &short})
	if err != nil || up.Version != 2 || up.Summary != short || up.Title != "Доступы" || len(up.Tags) != 1 {
		t.Fatalf("update = %+v, %v", up, err)
	}
	title := "Доступы стажёра"
	if up, _ = s.UpdateVaultItem(ctx, "u-liza", v.ID, domain.VaultPatch{Title: &title, Tags: []string{"доступы", "w1"}}); up.Version != 3 || len(up.Tags) != 2 {
		t.Errorf("second update = %+v", up)
	}
	back, err := s.RestoreVaultVersion(ctx, "u-liza", v.ID, 1)
	if err != nil || back.Version != 4 || back.Summary != "Семь доступов" || back.Title != "Доступы" {
		t.Errorf("restore = %+v, %v", back, err)
	}
	full, err := s.VaultItem(ctx, "u-liza", v.ID)
	if err != nil || len(full.Versions) != 4 || full.Versions[0].Version != 4 || full.Versions[3].Summary != "Семь доступов" {
		t.Errorf("versions = %+v, %v", full.Versions, err)
	}

	if all, _ := s.VaultItems(ctx, "u-liza", ""); len(all) != 2 || all[0].ID != v.ID {
		t.Errorf("list = %+v, want the last changed first", all)
	}
	if tagged, _ := s.VaultItems(ctx, "u-liza", "w1"); len(tagged) != 0 {
		t.Errorf("w1 tag after restore = %+v", tagged)
	}
	if others, _ := s.VaultItems(ctx, "u-anna", ""); len(others) != 0 {
		t.Errorf("another user sees %d records", len(others))
	}
	for name, err := range map[string]error{
		"get someone else's":     func() error { _, err := s.VaultItem(ctx, "u-anna", v.ID); return err }(),
		"get bad id":             func() error { _, err := s.VaultItem(ctx, "u-liza", "nope"); return err }(),
		"update someone else's":  func() error { _, err := s.UpdateVaultItem(ctx, "u-anna", v.ID, domain.VaultPatch{}); return err }(),
		"restore missing":        func() error { _, err := s.RestoreVaultVersion(ctx, "u-liza", v.ID, 9); return err }(),
		"restore someone else's": func() error { _, err := s.RestoreVaultVersion(ctx, "u-anna", v.ID, 1); return err }(),
		"delete someone else's":  s.DeleteVaultItem(ctx, "u-anna", v.ID),
		"delete bad id":          s.DeleteVaultItem(ctx, "u-liza", "nope"),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := s.DeleteVaultItem(ctx, "u-liza", v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.VaultItem(ctx, "u-liza", v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
}
