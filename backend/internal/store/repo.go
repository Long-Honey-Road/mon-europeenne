package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

var ErrNotFound = domain.ErrNotFound

// Store reads and writes application data; callers pass external user ids, never database uuids.
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// SeedOrg upserts teams and users by external id. Credentials stay with the auth service.
func (s *Store) SeedOrg(ctx context.Context, teams []domain.Team, users []domain.User) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, t := range teams {
			if _, err := tx.Exec(ctx, `
				INSERT INTO buddy.teams (external_id, name, unit, cluster, mattermost_channel)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (external_id) DO UPDATE SET name = EXCLUDED.name, unit = EXCLUDED.unit,
					cluster = EXCLUDED.cluster, mattermost_channel = EXCLUDED.mattermost_channel`,
				t.ID, t.Name, t.Unit, t.Cluster, t.MattermostChannel); err != nil {
				return fmt.Errorf("team %s: %w", t.ID, err)
			}
		}
		for _, u := range users {
			var teamID *string
			if u.TeamID != "" {
				var id string
				if err := tx.QueryRow(ctx, `SELECT id FROM buddy.teams WHERE external_id = $1`, u.TeamID).Scan(&id); err != nil {
					return fmt.Errorf("user %s: team %s: %w", u.ID, u.TeamID, err)
				}
				teamID = &id
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO buddy.users (external_id, login, password_hash, display_name, role_id, direction_id,
					team_id, repos, jira_projects, location, start_date)
				VALUES ($1, $2, '', $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (external_id) DO UPDATE SET login = EXCLUDED.login, display_name = EXCLUDED.display_name,
					role_id = EXCLUDED.role_id, direction_id = EXCLUDED.direction_id, team_id = EXCLUDED.team_id,
					repos = EXCLUDED.repos, jira_projects = EXCLUDED.jira_projects, location = EXCLUDED.location,
					start_date = EXCLUDED.start_date`,
				u.ID, u.Login, u.DisplayName, int16(u.Role), nullDirection(u.Direction), teamID,
				orEmpty(u.Repos), orEmpty(u.JiraProjects), u.Location, nullDate(u.StartDate)); err != nil {
				return fmt.Errorf("user %s: %w", u.ID, err)
			}
		}
		return nil
	})
}

const threadColumns = `t.id, u.external_id, t.type_id, t.title, t.pin, t.summary, t.created_at`

func (s *Store) CreateThread(ctx context.Context, userID string, typ domain.ThreadType, title string, pin domain.ThreadPin) (domain.Thread, error) {
	pinJSON, err := json.Marshal(pin)
	if err != nil {
		return domain.Thread{}, err
	}
	row := s.pool.QueryRow(ctx, `
		WITH t AS (
			INSERT INTO buddy.threads (user_id, type_id, title, pin)
			SELECT id, $2, $3, $4 FROM buddy.users WHERE external_id = $1
			RETURNING *
		)
		SELECT `+threadColumns+` FROM t JOIN buddy.users u ON u.id = t.user_id`,
		userID, int16(typ), title, pinJSON)
	return scanThread(row)
}

// Thread returns ErrNotFound for a missing thread and for someone else's.
func (s *Store) Thread(ctx context.Context, userID, id string) (domain.Thread, error) {
	if uuid.Validate(id) != nil {
		return domain.Thread{}, ErrNotFound
	}
	row := s.pool.QueryRow(ctx, `SELECT `+threadColumns+`
		FROM buddy.threads t JOIN buddy.users u ON u.id = t.user_id
		WHERE t.id = $1 AND u.external_id = $2`, id, userID)
	return scanThread(row)
}

// Threads lists the user's threads, newest first.
func (s *Store) Threads(ctx context.Context, userID string) ([]domain.Thread, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+threadColumns+`
		FROM buddy.threads t JOIN buddy.users u ON u.id = t.user_id
		WHERE u.external_id = $1 ORDER BY t.created_at DESC, t.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Thread
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) AddMessage(ctx context.Context, m domain.Message) (domain.Message, error) {
	if uuid.Validate(m.ThreadID) != nil {
		return domain.Message{}, ErrNotFound
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO buddy.messages (thread_id, role_id, content, blocks, attachments, job_id)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id, created_at`,
		m.ThreadID, int16(m.Role), m.Content, orArray(m.Blocks), orArray(m.Attachments), m.JobID,
	).Scan(&m.ID, &m.CreatedAt)
	return m, err
}

// Messages returns the last limit messages of a thread, oldest first.
func (s *Store) Messages(ctx context.Context, threadID string, limit int) ([]domain.Message, error) {
	if uuid.Validate(threadID) != nil {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, thread_id, role_id, content, blocks, attachments, job_id, created_at FROM (
			SELECT * FROM buddy.messages WHERE thread_id = $1 ORDER BY created_at DESC, id LIMIT $2
		) last ORDER BY created_at, id`, threadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Message
	for rows.Next() {
		var m domain.Message
		var role int16
		if err := rows.Scan(&m.ID, &m.ThreadID, &role, &m.Content, &m.Blocks, &m.Attachments, &m.JobID, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Role = domain.MessageRole(role)
		out = append(out, m)
	}
	return out, rows.Err()
}

// RateMessage stores a feedback event for an assistant message in the user's own thread, ErrNotFound otherwise.
func (s *Store) RateMessage(ctx context.Context, userID, messageID, value, note string) error {
	if uuid.Validate(messageID) != nil {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO buddy.events (actor_id, type_id, payload)
		SELECT u.id, $3, jsonb_build_object('message_id', m.id, 'thread_id', m.thread_id, 'value', $4::text, 'note', $5::text)
		FROM buddy.messages m JOIN buddy.threads t ON t.id = m.thread_id JOIN buddy.users u ON u.id = t.user_id
		WHERE m.id = $2 AND u.external_id = $1 AND m.role_id = $6`,
		userID, messageID, int16(domain.EventFeedback), value, note, int16(domain.MessageAssistant))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AppendEvents writes usage events; an unknown actor is stored as NULL.
func (s *Store) AppendEvents(ctx context.Context, actorID string, events []domain.Event) error {
	batch := &pgx.Batch{}
	for _, e := range events {
		batch.Queue(`INSERT INTO buddy.events (actor_id, type_id, payload)
			VALUES ((SELECT id FROM buddy.users WHERE external_id = $1), $2, $3)`,
			actorID, int16(e.Type), orObject(e.Payload))
	}
	return s.pool.SendBatch(ctx, batch).Close()
}

func scanThread(row pgx.Row) (domain.Thread, error) {
	var t domain.Thread
	var typ int16
	var pin []byte
	err := row.Scan(&t.ID, &t.UserID, &typ, &t.Title, &pin, &t.Summary, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Thread{}, ErrNotFound
	}
	if err != nil {
		return domain.Thread{}, err
	}
	t.Type = domain.ThreadType(typ)
	if err := json.Unmarshal(pin, &t.Pin); err != nil {
		return domain.Thread{}, fmt.Errorf("thread %s pin: %w", t.ID, err)
	}
	return t, nil
}

func nullDirection(d domain.Direction) *int16 {
	if d == 0 {
		return nil
	}
	v := int16(d)
	return &v
}

func nullDate(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orArray(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return raw
}

func orObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}
