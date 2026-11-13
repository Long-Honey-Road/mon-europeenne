package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// ReplaceDocuments makes the documents of one kind equal to docs: each is upserted with its chunks rewritten,
// documents of that kind missing from docs are removed.
func (s *Store) ReplaceDocuments(ctx context.Context, kind domain.SourceKind, docs []domain.KnowledgeDoc) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		keep := make([]string, 0, len(docs))
		for _, d := range docs {
			if d.Kind != kind {
				return fmt.Errorf("document %q: kind %v, want %v", d.ExternalID, d.Kind, kind)
			}
			// An unknown team must not turn a team document into a general one.
			var teamID *string
			if d.TeamID != "" {
				if err := tx.QueryRow(ctx, `SELECT id FROM buddy.teams WHERE external_id = $1`, d.TeamID).Scan(&teamID); err != nil {
					return fmt.Errorf("document %q: team %q: %w", d.ExternalID, d.TeamID, err)
				}
			}
			var id string
			err := tx.QueryRow(ctx, `
				INSERT INTO buddy.documents (source_kind_id, external_id, title, team_id, topic, url, is_test_instruction, fetched_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (source_kind_id, external_id) DO UPDATE SET
					title = EXCLUDED.title, team_id = EXCLUDED.team_id, topic = EXCLUDED.topic, url = EXCLUDED.url,
					is_test_instruction = EXCLUDED.is_test_instruction, fetched_at = EXCLUDED.fetched_at,
					version = buddy.documents.version + 1
				RETURNING id`,
				int16(kind), d.ExternalID, d.Title, teamID, d.Topic, d.URL, d.TestInstruction, d.FetchedAt).Scan(&id)
			if err != nil {
				return fmt.Errorf("document %q: %w", d.ExternalID, err)
			}
			if _, err := tx.Exec(ctx, `DELETE FROM buddy.chunks WHERE document_id = $1`, id); err != nil {
				return err
			}
			rows := make([][]any, 0, len(d.Chunks))
			for i, c := range d.Chunks {
				rows = append(rows, []any{id, i, c.Heading, c.Text})
			}
			if _, err := tx.CopyFrom(ctx, pgx.Identifier{"buddy", "chunks"}, []string{"document_id", "ord", "heading", "text"},
				pgx.CopyFromRows(rows)); err != nil {
				return fmt.Errorf("chunks of %q: %w", d.ExternalID, err)
			}
			keep = append(keep, d.ExternalID)
		}
		_, err := tx.Exec(ctx, `DELETE FROM buddy.documents WHERE source_kind_id = $1 AND external_id <> ALL($2)`, int16(kind), keep)
		return err
	})
}

// SearchChunks finds chunks for a free-text query, best first. Any word may match. Chunks with every word come
// first, then by rank; the user's own team documents weigh double, as they are the most specific. A team
// document is visible only to its team: the filter is part of the query.
func (s *Store) SearchChunks(ctx context.Context, teamID, query string, limit int) ([]domain.Hit, error) {
	rows, err := s.pool.Query(ctx, `
		WITH q AS (
			SELECT websearch_to_tsquery('russian', $2) AS q_all,
			       -- Cast, not to_tsquery: the lexemes are already stemmed and must not be stemmed twice.
			       replace(websearch_to_tsquery('russian', $2)::text, ' & ', ' | ')::tsquery AS q_any
		)
		SELECT c.id, d.external_id, d.source_kind_id, d.title, c.heading, c.text, d.url, d.topic,
		       d.team_id IS NOT NULL, d.is_test_instruction, d.fetched_at
		FROM q, buddy.chunks c
		JOIN buddy.documents d ON d.id = c.document_id
		LEFT JOIN buddy.teams t ON t.id = d.team_id
		WHERE c.tsv @@ q.q_any AND (d.team_id IS NULL OR t.external_id = $1)
		ORDER BY c.tsv @@ q.q_all DESC,
		         ts_rank_cd(c.tsv, q.q_any, 32) * CASE WHEN d.team_id IS NULL THEN 1 ELSE 2 END DESC,
		         c.id
		LIMIT $3`, teamID, withoutCompanyName(query), limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Hit, error) {
		var h domain.Hit
		var kind int16
		err := r.Scan(&h.ChunkID, &h.DocID, &kind, &h.Title, &h.Heading, &h.Text, &h.URL, &h.Topic,
			&h.TeamScoped, &h.TestInstruction, &h.FetchedAt)
		h.Kind = domain.SourceKind(kind)
		return h, err
	})
}

// companyWords are in nearly every document, so they rank noise above the asked topic.
var companyWords = map[string]bool{"авито": true, "авите": true, "авиту": true, "avito": true}

// withoutCompanyName drops the company's name from a query that has other words.
func withoutCompanyName(query string) string {
	words := strings.Fields(query)
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if !companyWords[strings.ToLower(strings.Trim(w, "?!.,:;«»\"'()"))] {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		return query
	}
	return strings.Join(kept, " ")
}
