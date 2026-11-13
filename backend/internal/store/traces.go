package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

func msDuration(ms int64) time.Duration { return time.Duration(ms) * time.Millisecond }

func nullID(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// SaveTrace stores one turn's trace; ThreadID must exist.
func (s *Store) SaveTrace(ctx context.Context, t domain.TurnTrace) error {
	violations, err := json.Marshal(orEmpty(t.Violations))
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO buddy.turn_traces (thread_id, question_id, answer_id, job_id, mode, duration_ms, system_prompt,
			steps, raw_text, final_text, violations, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		t.ThreadID, nullID(t.QuestionID), nullID(t.AnswerID), t.JobID, t.Mode, t.Duration.Milliseconds(),
		t.SystemPrompt, orArray(t.Steps), t.RawText, t.FinalText, violations, t.Error)
	return err
}

// TraceFilter narrows Traces; empty fields match everything.
type TraceFilter struct {
	UserID   string
	ThreadID string
	Limit    int
}

// Traces returns the latest traces first, with the question text and the thread's owner and type.
func (s *Store) Traces(ctx context.Context, f TraceFilter) ([]domain.TurnTrace, error) {
	if f.Limit <= 0 {
		f.Limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT tr.id, tr.thread_id, th.type_id, u.external_id, coalesce(tr.question_id::text, ''), coalesce(q.content, ''),
			coalesce(tr.answer_id::text, ''), tr.job_id, tr.mode, tr.duration_ms, tr.system_prompt, tr.steps,
			tr.raw_text, tr.final_text, tr.violations, tr.error, tr.created_at
		FROM buddy.turn_traces tr
		JOIN buddy.threads th ON th.id = tr.thread_id
		JOIN buddy.users u ON u.id = th.user_id
		LEFT JOIN buddy.messages q ON q.id = tr.question_id
		WHERE ($1 = '' OR u.external_id = $1) AND ($2 = '' OR tr.thread_id::text = $2)
		ORDER BY tr.created_at DESC LIMIT $3`, f.UserID, f.ThreadID, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TurnTrace
	for rows.Next() {
		var t domain.TurnTrace
		var typ int16
		var ms int64
		var violations []byte
		if err := rows.Scan(&t.ID, &t.ThreadID, &typ, &t.UserID, &t.QuestionID, &t.Question, &t.AnswerID, &t.JobID,
			&t.Mode, &ms, &t.SystemPrompt, &t.Steps, &t.RawText, &t.FinalText, &violations, &t.Error, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.ThreadType = domain.ThreadType(typ)
		t.Duration = msDuration(ms)
		if err := json.Unmarshal(violations, &t.Violations); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
