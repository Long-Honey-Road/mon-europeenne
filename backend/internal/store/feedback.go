package store

import (
	"context"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// FeedbackEntry is one rating of an answer with the question before it, for reading testers' comments.
type FeedbackEntry struct {
	At       time.Time
	Login    string
	Value    string
	Note     string
	Question string
	Answer   string
	ThreadID string
}

// Feedback lists the latest ratings, newest first; onlyNotes keeps those with a comment.
func (s *Store) Feedback(ctx context.Context, limit int, onlyNotes bool) ([]FeedbackEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.at, coalesce(u.login, ''), coalesce(e.payload->>'value', ''), coalesce(e.payload->>'note', ''),
		       coalesce((SELECT q.content FROM buddy.messages q
		                 WHERE q.thread_id = m.thread_id AND q.role_id = $2 AND q.created_at <= m.created_at
		                 ORDER BY q.created_at DESC LIMIT 1), ''),
		       m.content, m.thread_id::text
		FROM buddy.events e
		JOIN buddy.messages m ON m.id::text = e.payload->>'message_id'
		LEFT JOIN buddy.users u ON u.id = e.actor_id
		WHERE e.type_id = $1 AND (NOT $3 OR coalesce(e.payload->>'note', '') <> '')
		ORDER BY e.at DESC, e.id DESC
		LIMIT $4`, int16(domain.EventFeedback), int16(domain.MessageUser), onlyNotes, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FeedbackEntry
	for rows.Next() {
		var f FeedbackEntry
		if err := rows.Scan(&f.At, &f.Login, &f.Value, &f.Note, &f.Question, &f.Answer, &f.ThreadID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
