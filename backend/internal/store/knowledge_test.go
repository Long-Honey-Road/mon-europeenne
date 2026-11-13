package store

import (
	"context"
	"testing"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func knowledgeDocs() []domain.KnowledgeDoc {
	at := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	return []domain.KnowledgeDoc{
		{Kind: domain.SourceDoc, ExternalID: "general-review", Title: "Код-ревью", Topic: "code-review", TestInstruction: true, FetchedAt: at,
			Chunks: []domain.Chunk{{Heading: "Код-ревью / Аппрувы", Text: "Пулреквест сливается после двух аппрувов."}}},
		{Kind: domain.SourceDoc, ExternalID: "team-review", Title: "Ревью в команде", TeamID: "t-messenger", Topic: "code-review", TestInstruction: true, FetchedAt: at,
			Chunks: []domain.Chunk{{Heading: "Ревью / Аппрувы", Text: "Достаточно одного аппрува от владельца кода."}}},
		{Kind: domain.SourceDoc, ExternalID: "other-push", Title: "Push", TeamID: "t-other", Topic: "push", TestInstruction: true, FetchedAt: at,
			Chunks: []domain.Chunk{{Heading: "Push / Ретраи", Text: "Уведомления отправляются с ретраями."}}},
	}
}

func TestReplaceAndSearchDocuments(t *testing.T) {
	s := seeded(t)
	ctx := context.Background()
	if err := s.SeedOrg(ctx, []domain.Team{{ID: "t-other", Name: "Other"}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceDocuments(ctx, domain.SourceDoc, knowledgeDocs()); err != nil {
		t.Fatalf("ReplaceDocuments: %v", err)
	}

	hits, err := s.SearchChunks(ctx, "t-messenger", "Сколько нужно аппрувов на ревью?", 5)
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits = %+v, want the general and the team document", hits)
	}
	for _, h := range hits {
		if h.Kind != domain.SourceDoc || h.Topic != "code-review" || !h.TestInstruction || h.FetchedAt.IsZero() {
			t.Errorf("hit = %+v", h)
		}
		if h.TeamScoped != (h.DocID == "team-review") {
			t.Errorf("%s: team scoped = %v", h.DocID, h.TeamScoped)
		}
	}

	// Another team's document is invisible, an empty query finds nothing.
	if hits, _ := s.SearchChunks(ctx, "t-messenger", "уведомления ретраи", 5); len(hits) != 0 {
		t.Errorf("other team's document leaked: %+v", hits)
	}
	if hits, _ := s.SearchChunks(ctx, "t-other", "уведомления ретраи", 5); len(hits) != 1 {
		t.Errorf("own team's document = %+v", hits)
	}
	if hits, err := s.SearchChunks(ctx, "t-other", "", 5); err != nil || len(hits) != 0 {
		t.Errorf("empty query = %+v, %v", hits, err)
	}

	// Replace: a changed document is rewritten, a missing one removed.
	docs := knowledgeDocs()[:1]
	docs[0].Chunks = []domain.Chunk{{Heading: "Код-ревью", Text: "Теперь нужен один аппрув."}, {Heading: "Код-ревью / Сроки", Text: "Ответ за день."}}
	if err := s.ReplaceDocuments(ctx, domain.SourceDoc, docs); err != nil {
		t.Fatal(err)
	}
	if hits, _ := s.SearchChunks(ctx, "t-messenger", "аппрув", 5); len(hits) != 1 || hits[0].DocID != "general-review" {
		t.Errorf("after replace = %+v", hits)
	}
	var version, chunks int
	_ = s.pool.QueryRow(ctx, `SELECT d.version, count(c.id) FROM buddy.documents d JOIN buddy.chunks c ON c.document_id = d.id
		WHERE d.external_id = 'general-review' GROUP BY d.version`).Scan(&version, &chunks)
	if version != 2 || chunks != 2 {
		t.Errorf("version %d, chunks %d; want 2 and 2", version, chunks)
	}

	// The team's own document outranks a general one of the same words; the company name is noise.
	if hits, _ := s.SearchChunks(ctx, "t-messenger", "аппрув", 5); len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	if err := s.ReplaceDocuments(ctx, domain.SourceDoc, knowledgeDocs()); err != nil {
		t.Fatal(err)
	}
	if hits, _ := s.SearchChunks(ctx, "t-messenger", "аппрув в Авито?", 5); len(hits) != 2 || hits[0].DocID != "team-review" {
		t.Errorf("ranking = %+v, want the team document first", hits)
	}
	if hits, _ := s.SearchChunks(ctx, "t-messenger", "Авито", 5); len(hits) != 0 {
		t.Errorf("company name alone = %+v", hits)
	}

	// Other kinds are left alone; a wrong kind is refused.
	pb := []domain.KnowledgeDoc{{Kind: domain.SourcePlaybook, ExternalID: "values.md", Title: "Ценности", URL: "https://github.com/x", FetchedAt: time.Now(),
		Chunks: []domain.Chunk{{Heading: "Ценности", Text: "Упрощай."}}}}
	if err := s.ReplaceDocuments(ctx, domain.SourcePlaybook, pb); err != nil {
		t.Fatal(err)
	}
	if hits, _ := s.SearchChunks(ctx, "t-messenger", "упрощай аппрув", 5); len(hits) != 3 {
		t.Errorf("both kinds = %+v", hits)
	}
	if err := s.ReplaceDocuments(ctx, domain.SourceDoc, pb); err == nil {
		t.Error("wrong kind accepted")
	}
	ghost := knowledgeDocs()[2:]
	ghost[0].TeamID = "t-ghost"
	if err := s.ReplaceDocuments(ctx, domain.SourceDoc, ghost); err == nil {
		t.Error("document of an unknown team accepted")
	}
}
