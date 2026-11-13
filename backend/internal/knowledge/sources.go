package knowledge

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"google.golang.org/grpc"

	documentsv1 "github.com/Long-Honey-Road/mon-europeenne/backend/gen/mocks/documents/v1"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
)

// SeedDocs serves the team documents from seed when no documents service is configured.
type SeedDocs []seed.Doc

func (SeedDocs) Name() string            { return "seed documents" }
func (SeedDocs) Kind() domain.SourceKind { return domain.SourceDoc }

func (s SeedDocs) Fetch(context.Context) ([]Document, error) {
	now := time.Now().UTC()
	out := make([]Document, 0, len(s))
	for _, d := range s {
		out = append(out, Document{ID: d.ID, Title: d.Title, TeamID: d.TeamID, Topic: d.Topic, Markdown: d.Markdown, TestInstruction: true, FetchedAt: now})
	}
	return out, nil
}

// MockDocs reads the team wiki through the documents service.
type MockDocs struct {
	c documentsv1.DocumentsServiceClient
}

func NewMockDocs(conn grpc.ClientConnInterface) MockDocs {
	return MockDocs{c: documentsv1.NewDocumentsServiceClient(conn)}
}

func (MockDocs) Name() string            { return "documents service" }
func (MockDocs) Kind() domain.SourceKind { return domain.SourceDoc }

func (m MockDocs) Fetch(ctx context.Context) ([]Document, error) {
	list, err := m.c.ListDocuments(ctx, &documentsv1.ListDocumentsRequest{})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]Document, 0, len(list.GetDocuments()))
	for _, info := range list.GetDocuments() {
		doc, err := m.c.GetDocument(ctx, &documentsv1.GetDocumentRequest{DocumentId: info.GetId()})
		if err != nil {
			return nil, fmt.Errorf("document %q: %w", info.GetId(), err)
		}
		out = append(out, Document{
			ID: info.GetId(), Title: info.GetTitle(), TeamID: info.GetTeamId(), Topic: info.GetTopic(),
			Markdown: doc.GetMarkdown(), TestInstruction: info.GetTestInstruction(), FetchedAt: now,
		})
	}
	return out, nil
}

// maxArchiveBytes bounds the Playbook download; the repository is about 150 KB packed.
const maxArchiveBytes = 20 << 20

// Playbook reads the open Avito Playbook from a GitHub tarball at a fixed ref. Nothing is stored in the repo:
// the text lives only in the index, with a link to the file on GitHub.
type Playbook struct {
	Repo string
	Ref  string
	// CodeloadURL is https://codeload.github.com unless a test points it elsewhere.
	CodeloadURL string
	HTTP        *http.Client
}

func (Playbook) Name() string            { return "avito playbook" }
func (Playbook) Kind() domain.SourceKind { return domain.SourcePlaybook }

func (p Playbook) Fetch(ctx context.Context) ([]Document, error) {
	base := p.CodeloadURL
	if base == "" {
		base = "https://codeload.github.com"
	}
	client := p.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+p.Repo+"/tar.gz/"+p.Ref, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s@%s: %s", p.Repo, p.Ref, resp.Status)
	}
	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxArchiveBytes))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var out []Document
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		// Paths are "<repo>-<sha>/file.md"; the first part is the archive's own folder.
		_, rel, ok := strings.Cut(h.Name, "/")
		if h.Typeflag != tar.TypeReg || !ok || !strings.EqualFold(path.Ext(rel), ".md") {
			continue
		}
		raw, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		md := string(raw)
		out = append(out, Document{
			ID: rel, Title: titleOf(md, rel), Markdown: md, FetchedAt: now,
			URL: "https://github.com/" + p.Repo + "/blob/" + p.Ref + "/" + rel,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no markdown in %s@%s", p.Repo, p.Ref)
	}
	return out, nil
}

// titleOf is the first top-level heading, or the file name.
func titleOf(md, file string) string {
	for _, line := range strings.Split(md, "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	return strings.TrimSuffix(path.Base(file), path.Ext(file))
}
