package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// FocusActivity is how much a thread talked about one sphere.
type FocusActivity struct {
	Focus    domain.Focus
	Messages int
	Last     time.Time
}

// FocusMessages returns the last limit messages of a thread about one sphere, created before the given time,
// oldest first.
func (s *Store) FocusMessages(ctx context.Context, threadID string, f domain.Focus, before time.Time, limit int) ([]domain.Message, error) {
	if uuid.Validate(threadID) != nil {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+messageColumns+` FROM (
			SELECT * FROM buddy.messages
			WHERE thread_id = $1 AND focus_kind_id = $2 AND focus_id = $3 AND created_at < $4
			ORDER BY created_at DESC, id LIMIT $5
		) last ORDER BY created_at, id`, threadID, int16(f.Kind), f.ID, before, limit)
	if err != nil {
		return nil, err
	}
	return collectMessages(rows)
}

// ThreadFoci lists the spheres a thread talked about, most recent first.
func (s *Store) ThreadFoci(ctx context.Context, threadID string) ([]FocusActivity, error) {
	if uuid.Validate(threadID) != nil {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `
		SELECT focus_kind_id, focus_id, count(*), max(created_at) FROM buddy.messages
		WHERE thread_id = $1 AND focus_kind_id IS NOT NULL
		GROUP BY focus_kind_id, focus_id ORDER BY max(created_at) DESC`, threadID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (FocusActivity, error) {
		var a FocusActivity
		var kind int16
		err := r.Scan(&kind, &a.Focus.ID, &a.Messages, &a.Last)
		a.Focus.Kind = domain.FocusKind(kind)
		return a, err
	})
}

// CountAfter counts messages newer than after: of one sphere, or of the whole thread for a zero focus.
func (s *Store) CountAfter(ctx context.Context, threadID string, f domain.Focus, after time.Time) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM buddy.messages
		WHERE thread_id = $1 AND created_at > $2 AND ($3 = 0 OR (focus_kind_id = $3 AND focus_id = $4))`,
		threadID, after, int16(f.Kind), f.ID).Scan(&n)
	return n, err
}

// FocusSummary returns a sphere's summary and the time of the newest message it covers; empty without one.
func (s *Store) FocusSummary(ctx context.Context, threadID string, f domain.Focus) (string, time.Time, error) {
	var summary string
	var until time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT summary, covered_until FROM buddy.focus_summaries
		WHERE thread_id = $1 AND focus_kind_id = $2 AND focus_id = $3`, threadID, int16(f.Kind), f.ID).Scan(&summary, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, nil
	}
	return summary, until, err
}

func (s *Store) SaveFocusSummary(ctx context.Context, threadID string, f domain.Focus, summary string, until time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO buddy.focus_summaries (thread_id, focus_kind_id, focus_id, summary, covered_until)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (thread_id, focus_kind_id, focus_id) DO UPDATE SET
			summary = EXCLUDED.summary, covered_until = EXCLUDED.covered_until, updated_at = now()`,
		threadID, int16(f.Kind), f.ID, summary, until)
	return err
}

func (s *Store) SaveThreadSummary(ctx context.Context, threadID, summary string, until time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE buddy.threads SET summary = $2, summary_until = $3 WHERE id = $1`,
		threadID, summary, until)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ThreadSummary returns the thread's summary and the time of the newest message it covers.
func (s *Store) ThreadSummary(ctx context.Context, threadID string) (string, time.Time, error) {
	if uuid.Validate(threadID) != nil {
		return "", time.Time{}, ErrNotFound
	}
	var summary string
	var until *time.Time
	err := s.pool.QueryRow(ctx, `SELECT summary, summary_until FROM buddy.threads WHERE id = $1`, threadID).Scan(&summary, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, ErrNotFound
	}
	if until == nil {
		return summary, time.Time{}, err
	}
	return summary, *until, err
}
