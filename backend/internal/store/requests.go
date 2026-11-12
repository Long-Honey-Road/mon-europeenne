package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// Mirror of the corp-requests service; part of the removable requests module (FEATURE_REQUESTS).

const requestColumns = `r.id, u.external_id, coalesce(r.thread_id::text, ''), r.type_key, r.external_id, r.status,
	r.fields, r.approver, r.history, r.updated_at`

// SaveRequest upserts by the service's request id; ThreadID is optional.
func (s *Store) SaveRequest(ctx context.Context, r domain.Request) error {
	var thread *string
	if r.ThreadID != "" {
		thread = &r.ThreadID
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO buddy.requests (user_id, thread_id, type_key, external_id, status, fields, approver, history)
		SELECT u.id, $2, $3, $4, $5, $6, $7, $8 FROM buddy.users u WHERE u.external_id = $1
		ON CONFLICT (external_id) DO UPDATE SET status = EXCLUDED.status, fields = EXCLUDED.fields,
			approver = EXCLUDED.approver, history = EXCLUDED.history, updated_at = now()`,
		r.UserID, thread, r.TypeKey, r.ExternalID, r.Status, orObject(r.Fields), orObject(r.Approver), orArray(r.History))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Request returns a request and the team of its author, for team-level access checks.
func (s *Store) Request(ctx context.Context, externalID string) (domain.Request, string, error) {
	var r domain.Request
	var team string
	err := s.pool.QueryRow(ctx, `SELECT `+requestColumns+`, coalesce(t.external_id, '')
		FROM buddy.requests r JOIN buddy.users u ON u.id = r.user_id LEFT JOIN buddy.teams t ON t.id = u.team_id
		WHERE r.external_id = $1`, externalID).
		Scan(&r.ID, &r.UserID, &r.ThreadID, &r.TypeKey, &r.ExternalID, &r.Status, &r.Fields, &r.Approver, &r.History,
			&r.UpdatedAt, &team)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Request{}, "", ErrNotFound
	}
	return r, team, err
}

// RequestsOf lists a user's requests, last updated first.
func (s *Store) RequestsOf(ctx context.Context, userID string) ([]domain.Request, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+requestColumns+`
		FROM buddy.requests r JOIN buddy.users u ON u.id = r.user_id
		WHERE u.external_id = $1 ORDER BY r.updated_at DESC, r.external_id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Request
	for rows.Next() {
		var r domain.Request
		if err := rows.Scan(&r.ID, &r.UserID, &r.ThreadID, &r.TypeKey, &r.ExternalID, &r.Status, &r.Fields,
			&r.Approver, &r.History, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
