package main

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/knowledge"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/tools"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

func toolNames(t *testing.T, st *store.Store, requestsOn bool, thread domain.ThreadType) []string {
	t.Helper()
	data, err := seed.Load(seed.Files)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	var search tools.Searcher
	if st != nil {
		search = knowledge.New(st)
	}
	reg, err := agentTools(data, data.Directory(), st, search, nil, requestsOn)
	if err != nil {
		t.Fatalf("agentTools: %v", err)
	}
	var names []string
	for _, tl := range reg.Available(domain.Principal{UserID: "u-liza", Role: domain.RoleIntern}, thread) {
		names = append(names, tl.Name)
	}
	return names
}

func TestAgentToolsDependOnDatabaseAndFlag(t *testing.T) {
	cfg := stackConfig(t)
	pool, err := store.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()
	st := store.New(pool)

	if got := toolNames(t, nil, true, domain.ThreadGeneral); !slices.Equal(got, []string{"general_knowledge", "report_not_found", "team_members", "who_to_ask"}) {
		t.Errorf("without a database: %v", got)
	}
	want := []string{"focus_sphere", "general_knowledge", "get_plan", "get_request_status", "get_request_types", "highlight_spheres", "list_my_requests", "prepare_request", "report_not_found", "search_docs", "team_members", "vault_list", "vault_save", "who_to_ask"}
	if got := toolNames(t, st, true, domain.ThreadGeneral); !slices.Equal(got, want) {
		t.Errorf("with requests: %v", got)
	}
	if got := toolNames(t, st, false, domain.ThreadGeneral); !slices.Equal(got, []string{"focus_sphere", "general_knowledge", "get_plan", "highlight_spheres", "report_not_found", "search_docs", "team_members", "vault_list", "vault_save", "who_to_ask"}) {
		t.Errorf("FEATURE_REQUESTS off: %v", got)
	}
	if got := toolNames(t, st, true, domain.ThreadVault); !slices.Contains(got, "vault_update") || slices.Contains(got, "pin_note") {
		t.Errorf("vault thread: %v", got)
	}
	if got := toolNames(t, st, true, domain.ThreadPlan); !slices.Contains(got, "update_item_status") || !slices.Contains(got, "pin_note") {
		t.Errorf("plan thread: %v", got)
	}
}

func TestStandRulesOnlyWithAFile(t *testing.T) {
	if standRules("") != nil {
		t.Error("rules without a file")
	}
	if r := standRules(filepath.Join(t.TempDir(), "none.md")); r == nil || r() != "" {
		t.Error("a missing file must give empty rules")
	}
}
