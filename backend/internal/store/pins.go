package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

const pinColumns = `p.id, u.external_id, p.focus_kind_id, p.focus_id, p.title, p.text, p.sources,
	coalesce(p.message_id::text, ''), p.created_at, p.updated_at`

func scanPin(r pgx.Row) (domain.Pin, error) {
	var p domain.Pin
	var kind int16
	err := r.Scan(&p.ID, &p.UserID, &kind, &p.Focus.ID, &p.Title, &p.Text, &p.Sources, &p.MessageID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Pin{}, ErrNotFound
	}
	p.Focus.Kind = domain.FocusKind(kind)
	return p, err
}

func (s *Store) CreatePin(ctx context.Context, p domain.Pin) (domain.Pin, error) {
	var msg *string
	if p.MessageID != "" {
		msg = &p.MessageID
	}
	return scanPin(s.pool.QueryRow(ctx, `
		WITH p AS (
			INSERT INTO buddy.pins (user_id, focus_kind_id, focus_id, title, text, sources, message_id)
			SELECT id, $2, $3, $4, $5, $6, $7 FROM buddy.users WHERE external_id = $1
			RETURNING *
		)
		SELECT `+pinColumns+` FROM p JOIN buddy.users u ON u.id = p.user_id`,
		p.UserID, int16(p.Focus.Kind), p.Focus.ID, p.Title, p.Text, orArray(p.Sources), msg))
}

// Pins lists the user's pins, oldest first; a zero focus lists all of them.
func (s *Store) Pins(ctx context.Context, userID string, f domain.Focus) ([]domain.Pin, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+pinColumns+`
		FROM buddy.pins p JOIN buddy.users u ON u.id = p.user_id
		WHERE u.external_id = $1 AND ($2 = 0 OR (p.focus_kind_id = $2 AND p.focus_id = $3))
		ORDER BY p.created_at, p.id`, userID, int16(f.Kind), f.ID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Pin, error) { return scanPin(r) })
}

// UpdatePin changes the title and text of the user's own pin; someone else's pin is ErrNotFound.
func (s *Store) UpdatePin(ctx context.Context, userID, id, title, text string) (domain.Pin, error) {
	if uuid.Validate(id) != nil {
		return domain.Pin{}, ErrNotFound
	}
	return scanPin(s.pool.QueryRow(ctx, `
		WITH p AS (
			UPDATE buddy.pins SET title = $3, text = $4, updated_at = now()
			WHERE id = $2 AND user_id = (SELECT id FROM buddy.users WHERE external_id = $1)
			RETURNING *
		)
		SELECT `+pinColumns+` FROM p JOIN buddy.users u ON u.id = p.user_id`, userID, id, title, text))
}

func (s *Store) DeletePin(ctx context.Context, userID, id string) error {
	if uuid.Validate(id) != nil {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM buddy.pins
		WHERE id = $2 AND user_id = (SELECT id FROM buddy.users WHERE external_id = $1)`, userID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// OwnMessage returns a message from one of the user's threads; anyone else's is ErrNotFound.
func (s *Store) OwnMessage(ctx context.Context, userID, id string) (domain.Message, error) {
	if uuid.Validate(id) != nil {
		return domain.Message{}, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT `+messageColumnsOf("m")+`
		FROM buddy.messages m JOIN buddy.threads t ON t.id = m.thread_id JOIN buddy.users u ON u.id = t.user_id
		WHERE m.id = $1 AND u.external_id = $2`, id, userID)
	if err != nil {
		return domain.Message{}, err
	}
	msgs, err := collectMessages(rows)
	if err != nil {
		return domain.Message{}, err
	}
	if len(msgs) == 0 {
		return domain.Message{}, ErrNotFound
	}
	return msgs[0], nil
}
