package knowledge

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/mocks/mockstest"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

type memStore struct {
	docs    map[domain.SourceKind][]domain.KnowledgeDoc
	hits    []domain.Hit
	failKey domain.SourceKind
	team    string
}

func (m *memStore) ReplaceDocuments(_ context.Context, kind domain.SourceKind, docs []domain.KnowledgeDoc) error {
	if kind == m.failKey {
		return errors.New("db down")
	}
	m.docs[kind] = docs
	return nil
}

func (m *memStore) SearchChunks(_ context.Context, teamID, _ string, _ int) ([]domain.Hit, error) {
	m.team = teamID
	if m.failKey != 0 {
		return nil, errors.New("db down")
	}
	return m.hits, nil
}

type failing struct{}

func (failing) Name() string                              { return "broken" }
func (failing) Kind() domain.SourceKind                   { return domain.SourcePlaybook }
func (failing) Fetch(context.Context) ([]Document, error) { return nil, errors.New("offline") }

func seedData(t *testing.T) seed.Data {
	t.Helper()
	d, err := seed.Load(seed.Files)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSyncIndexesSourcesAndSurvivesAFailingOne(t *testing.T) {
	st := &memStore{docs: map[domain.SourceKind][]domain.KnowledgeDoc{}}
	svc := New(st, failing{}, SeedDocs(seedData(t).Docs))
	err := svc.Sync(t.Context())
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("Sync error = %v, want the failing source named", err)
	}
	docs := st.docs[domain.SourceDoc]
	if len(docs) < 15 {
		t.Fatalf("indexed %d docs", len(docs))
	}
	for _, d := range docs {
		if !d.TestInstruction || len(d.Chunks) == 0 || d.FetchedAt.IsZero() || !strings.HasPrefix(d.Chunks[0].Heading, d.Title) {
			t.Errorf("doc %s = %+v", d.ExternalID, d)
		}
	}
	st.failKey = domain.SourceDoc
	if err := New(st, SeedDocs(nil)).Sync(t.Context()); err == nil {
		t.Error("store failure not reported")
	}
}

func TestSearchPassesTeamAndFindsConflicts(t *testing.T) {
	st := &memStore{hits: []domain.Hit{
		{DocID: "team-review", Kind: domain.SourceDoc, Topic: "code-review", TeamScoped: true},
		{DocID: "general-review", Kind: domain.SourceDoc, Topic: "code-review"},
		{DocID: "general-review", Kind: domain.SourceDoc, Topic: "code-review"},
		{DocID: "general-git", Kind: domain.SourceDoc, Topic: "git"},
		{DocID: "values.md", Kind: domain.SourcePlaybook},
	}}
	hits, conflicts, err := New(st).Search(t.Context(), domain.Principal{TeamID: "t-messenger"}, "ревью", 5)
	if err != nil || len(hits) != 5 || st.team != "t-messenger" {
		t.Fatalf("Search = %d hits, %v, team %q", len(hits), err, st.team)
	}
	want := []domain.ConflictCandidate{{DocA: "general-review", DocB: "team-review", Topic: "code-review"}}
	if len(conflicts) != 1 || conflicts[0] != want[0] {
		t.Errorf("conflicts = %+v, want %+v", conflicts, want)
	}
	st.failKey = domain.SourceDoc
	if _, _, err := New(st).Search(t.Context(), domain.Principal{}, "x", 1); err == nil {
		t.Error("store error swallowed")
	}
}

func TestMockDocs(t *testing.T) {
	conn, srv := mockstest.Start(t, mocks.Chaos{})
	src := NewMockDocs(conn)
	docs, err := src.Fetch(t.Context())
	if err != nil || len(docs) < 15 || src.Kind() != domain.SourceDoc || src.Name() == "" {
		t.Fatalf("Fetch = %d, %v", len(docs), err)
	}
	for _, d := range docs {
		if d.Markdown == "" || !d.TestInstruction {
			t.Errorf("%s = %+v", d.ID, d)
		}
	}
	srv.GRPC.Stop()
	if _, err := src.Fetch(t.Context()); err == nil {
		t.Error("Fetch with the service down: no error")
	}
}

func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "playbook-abc/", Typeflag: tar.TypeDir, Mode: 0o755})
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestPlaybook(t *testing.T) {
	archive := tarball(t, map[string]string{
		"playbook-abc/mission-and-values.md": "# Миссия\n\nТекст.\n\n# Ценности\n\n## Упрощай\nДелай проще.",
		"playbook-abc/notitle.md":            "без заголовка",
		"playbook-abc/logo.png":              "png",
		"stray.md":                           "outside the archive folder",
	})
	var gotPath string
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write(archive)
	}))
	t.Cleanup(web.Close)

	src := Playbook{Repo: "avito-tech/playbook", Ref: "abc", CodeloadURL: web.URL}
	docs, err := src.Fetch(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/avito-tech/playbook/tar.gz/abc" || src.Kind() != domain.SourcePlaybook || src.Name() == "" {
		t.Errorf("path %q", gotPath)
	}
	if len(docs) != 2 {
		t.Fatalf("docs = %+v, want the two markdown files", docs)
	}
	byID := map[string]Document{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	m := byID["mission-and-values.md"]
	if m.Title != "Миссия" || m.URL != "https://github.com/avito-tech/playbook/blob/abc/mission-and-values.md" || m.TestInstruction || m.FetchedAt.IsZero() {
		t.Errorf("mission = %+v", m)
	}
	if byID["notitle.md"].Title != "notitle" {
		t.Errorf("title fallback = %q", byID["notitle.md"].Title)
	}
}

func TestPlaybookFailures(t *testing.T) {
	serve := func(status int, body []byte) string {
		web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write(body)
		}))
		t.Cleanup(web.Close)
		return web.URL
	}
	for name, base := range map[string]string{
		"not found":   serve(http.StatusNotFound, nil),
		"not gzip":    serve(http.StatusOK, []byte("html")),
		"no markdown": serve(http.StatusOK, tarball(t, map[string]string{"p/a.txt": "x"})),
		"broken tar":  serve(http.StatusOK, tarball(t, map[string]string{"p/a.md": "x"})[:40]),
		"bad url":     "http://bad host",
		"no server":   "http://127.0.0.1:1",
	} {
		if _, err := (Playbook{Repo: "r", Ref: "x", CodeloadURL: base}).Fetch(t.Context()); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
