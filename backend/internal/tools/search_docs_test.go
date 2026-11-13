package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

type fakeSearcher struct {
	hits      []domain.Hit
	conflicts []domain.ConflictCandidate
	err       error
	gotLimit  int
	gotTeam   string
}

func (f *fakeSearcher) Search(_ context.Context, p domain.Principal, _ string, limit int) ([]domain.Hit, []domain.ConflictCandidate, error) {
	f.gotLimit, f.gotTeam = limit, p.TeamID
	return f.hits, f.conflicts, f.err
}

func callSearch(t *testing.T, s Searcher, args string) (searchDocsOut, error) {
	t.Helper()
	tool, err := SearchDocs(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(tool.Roles) != len(allRoles) || len(tool.Threads) != len(allThreads) {
		t.Errorf("search_docs must be open to every role and thread")
	}
	reg, err := NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}
	p := domain.Principal{UserID: "u-liza", Role: domain.RoleIntern, TeamID: "t-messenger"}
	raw, err := reg.Call(context.Background(), p, domain.ThreadGeneral, "search_docs", json.RawMessage(args))
	var out searchDocsOut
	if err == nil {
		_ = json.Unmarshal(raw, &out)
	}
	return out, err
}

func TestSearchDocsShapesFragments(t *testing.T) {
	at := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	f := &fakeSearcher{
		hits: []domain.Hit{
			{ChunkID: "c1", DocID: "messenger-code-review", Kind: domain.SourceDoc, Heading: "Ревью в Messenger / Аппрувы", Text: "Один аппрув", TeamScoped: true, TestInstruction: true},
			{ChunkID: "c2", DocID: "values.md", Kind: domain.SourcePlaybook, Heading: "Ценности", Text: "Упрощай", URL: "https://github.com/x", FetchedAt: at},
		},
		conflicts: []domain.ConflictCandidate{{DocA: "general-code-review", DocB: "messenger-code-review", Topic: "code-review"}},
	}
	out, err := callSearch(t, f, `{"query":"аппрувы","limit":50}`)
	if err != nil {
		t.Fatal(err)
	}
	if f.gotLimit != maxSearchLimit || f.gotTeam != "t-messenger" {
		t.Errorf("limit %d, team %q", f.gotLimit, f.gotTeam)
	}
	if len(out.Sources) != 2 {
		t.Fatalf("sources = %+v", out.Sources)
	}
	team, pb := out.Sources[0], out.Sources[1]
	if team.Scope != "team" || team.Kind != "doc" || !team.TestInstruction || team.Title != "Ревью в Messenger / Аппрувы" || team.FetchedAt != "" {
		t.Errorf("team fragment = %+v", team)
	}
	if pb.Scope != "general" || pb.Kind != "playbook" || pb.FetchedAt != "2026-11-14" || pb.URL == "" {
		t.Errorf("playbook fragment = %+v", pb)
	}
	if len(out.ConflictCandidates) != 1 || !strings.Contains(out.Note, "Playbook") || !strings.Contains(out.Note, "conflict_candidates") {
		t.Errorf("conflicts %+v, note %q", out.ConflictCandidates, out.Note)
	}
}

func TestSearchDocsDefaultsAndErrors(t *testing.T) {
	f := &fakeSearcher{}
	out, err := callSearch(t, f, `{"query":"ничего"}`)
	if err != nil || f.gotLimit != defaultSearchLimit || out.Sources == nil || len(out.Sources) != 0 || out.Note != "" {
		t.Errorf("empty search = %+v, %v, limit %d", out, err, f.gotLimit)
	}
	if _, err := callSearch(t, &fakeSearcher{err: errors.New("db down")}, `{"query":"x"}`); err == nil {
		t.Error("searcher error swallowed")
	}
}
