// Package knowledge indexes documents from their sources and searches them with the caller's team rights.
package knowledge

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Document is a source document before chunking.
type Document struct {
	ID    string
	Title string
	// TeamID is the team's external id; empty for everyone.
	TeamID          string
	Topic           string
	URL             string
	Markdown        string
	TestInstruction bool
	FetchedAt       time.Time
}

// Source fills one kind of documents.
type Source interface {
	Name() string
	Kind() domain.SourceKind
	Fetch(ctx context.Context) ([]Document, error)
}

type Store interface {
	ReplaceDocuments(ctx context.Context, kind domain.SourceKind, docs []domain.KnowledgeDoc) error
	SearchChunks(ctx context.Context, teamID, query string, limit int) ([]domain.Hit, error)
}

type Service struct {
	store   Store
	sources []Source
}

func New(store Store, sources ...Source) *Service {
	return &Service{store: store, sources: sources}
}

// Sync replaces the indexed documents of every source. A failing source keeps its previous copy and does not
// stop the others; the errors are returned together.
func (s *Service) Sync(ctx context.Context) error {
	var errs []error
	for _, src := range s.sources {
		docs, err := src.Fetch(ctx)
		if err == nil {
			err = s.store.ReplaceDocuments(ctx, src.Kind(), index(src.Kind(), docs))
		}
		if err != nil {
			slog.Warn("knowledge source failed, keeping the previous copy", "source", src.Name(), "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", src.Name(), err))
			continue
		}
		slog.Info("knowledge source synced", "source", src.Name(), "documents", len(docs))
	}
	return errors.Join(errs...)
}

func index(kind domain.SourceKind, docs []Document) []domain.KnowledgeDoc {
	out := make([]domain.KnowledgeDoc, 0, len(docs))
	for _, d := range docs {
		out = append(out, domain.KnowledgeDoc{
			Kind: kind, ExternalID: d.ID, Title: d.Title, TeamID: d.TeamID, Topic: d.Topic, URL: d.URL,
			TestInstruction: d.TestInstruction, FetchedAt: d.FetchedAt, Chunks: Split(d.Title, d.Markdown),
		})
	}
	return out
}

// Search returns chunks the caller may see, best first, and pairs of general and team documents of one topic.
func (s *Service) Search(ctx context.Context, p domain.Principal, query string, limit int) ([]domain.Hit, []domain.ConflictCandidate, error) {
	hits, err := s.store.SearchChunks(ctx, p.TeamID, query, limit)
	if err != nil {
		return nil, nil, err
	}
	return hits, conflicts(hits), nil
}

func conflicts(hits []domain.Hit) []domain.ConflictCandidate {
	general, team := map[string]string{}, map[string]string{}
	for _, h := range hits {
		if h.Kind != domain.SourceDoc || h.Topic == "" {
			continue
		}
		side := general
		if h.TeamScoped {
			side = team
		}
		if _, seen := side[h.Topic]; !seen {
			side[h.Topic] = h.DocID
		}
	}
	var out []domain.ConflictCandidate
	done := map[string]bool{}
	for _, h := range hits {
		if a, b := general[h.Topic], team[h.Topic]; a != "" && b != "" && !done[h.Topic] {
			done[h.Topic] = true
			out = append(out, domain.ConflictCandidate{DocA: a, DocB: b, Topic: h.Topic})
		}
	}
	return out
}
