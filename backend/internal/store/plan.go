package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

// visibleItem is true for plan items meant for everyone or for the user's direction and team.
const visibleItem = `(i.direction_id IS NULL OR i.direction_id = u.direction_id) AND (i.team_id IS NULL OR i.team_id = u.team_id)`

// SeedPlan upserts items by key and replaces their dependencies. Statuses are only added, never overwritten,
// so reseeding keeps the users' progress.
func (s *Store) SeedPlan(ctx context.Context, items []domain.PlanItem, statuses []domain.PlanStatus) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, it := range items {
			var teamID *string
			if it.TeamID != "" {
				var id string
				if err := tx.QueryRow(ctx, `SELECT id FROM buddy.teams WHERE external_id = $1`, it.TeamID).Scan(&id); err != nil {
					return fmt.Errorf("plan item %s: team %s: %w", it.Key, it.TeamID, err)
				}
				teamID = &id
			}
			mapJSON, err := json.Marshal(it.Map)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO buddy.plan_items (key, title, practice, period_id, direction_id, team_id, sources, map)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (key) DO UPDATE SET title = EXCLUDED.title, practice = EXCLUDED.practice,
					period_id = EXCLUDED.period_id, direction_id = EXCLUDED.direction_id, team_id = EXCLUDED.team_id,
					sources = EXCLUDED.sources, map = EXCLUDED.map`,
				it.Key, it.Title, it.Practice, int16(it.Period), nullDirection(it.Direction), teamID,
				orEmpty(it.Sources), mapJSON); err != nil {
				return fmt.Errorf("plan item %s: %w", it.Key, err)
			}
		}
		for _, it := range items {
			if _, err := tx.Exec(ctx, `DELETE FROM buddy.plan_item_dependencies
				WHERE item_id = (SELECT id FROM buddy.plan_items WHERE key = $1)`, it.Key); err != nil {
				return err
			}
			for _, dep := range it.DependsOn {
				if _, err := tx.Exec(ctx, `
					INSERT INTO buddy.plan_item_dependencies (item_id, depends_on_id)
					VALUES ((SELECT id FROM buddy.plan_items WHERE key = $1), (SELECT id FROM buddy.plan_items WHERE key = $2))`,
					it.Key, dep); err != nil {
					return fmt.Errorf("plan item %s depends on %s: %w", it.Key, dep, err)
				}
			}
		}
		for _, st := range statuses {
			if _, err := tx.Exec(ctx, `
				INSERT INTO buddy.plan_status (user_id, item_id, status_id, note)
				SELECT u.id, i.id, $3, $4 FROM buddy.users u, buddy.plan_items i
				WHERE u.external_id = $1 AND i.key = $2
				ON CONFLICT (user_id, item_id) DO NOTHING`,
				st.UserID, st.ItemID, int16(st.Status), st.Note); err != nil {
				return fmt.Errorf("plan status %s/%s: %w", st.UserID, st.ItemID, err)
			}
		}
		return nil
	})
}

// Plan returns the user's plan in period order; items without a status row are todo.
func (s *Store) Plan(ctx context.Context, userID string) ([]domain.PlanEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.key, i.title, i.practice, i.period_id, coalesce(i.direction_id, 0), coalesce(t.external_id, ''),
			i.sources, i.map, coalesce(ps.status_id, $2), coalesce(ps.note, ''),
			array(SELECT d.key FROM buddy.plan_item_dependencies pd JOIN buddy.plan_items d ON d.id = pd.depends_on_id
				WHERE pd.item_id = i.id ORDER BY d.key)
		FROM buddy.plan_items i
		JOIN buddy.users u ON u.external_id = $1
		LEFT JOIN buddy.teams t ON t.id = i.team_id
		LEFT JOIN buddy.plan_status ps ON ps.item_id = i.id AND ps.user_id = u.id
		WHERE `+visibleItem+`
		ORDER BY i.period_id, (i.map->>'x')::numeric NULLS FIRST, (i.map->>'y')::numeric NULLS FIRST, i.key`,
		userID, int16(domain.ItemTodo))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PlanEntry
	for rows.Next() {
		var e domain.PlanEntry
		var period, direction, status int16
		var mapJSON []byte
		if err := rows.Scan(&e.Item.Key, &e.Item.Title, &e.Item.Practice, &period, &direction, &e.Item.TeamID,
			&e.Item.Sources, &mapJSON, &status, &e.Note, &e.Item.DependsOn); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(mapJSON, &e.Item.Map); err != nil {
			return nil, fmt.Errorf("plan item %s map: %w", e.Item.Key, err)
		}
		e.Item.Period, e.Item.Direction, e.Status = domain.PlanPeriod(period), domain.Direction(direction), domain.PlanItemStatus(status)
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetItemStatus returns ErrNotFound for items outside the user's plan.
func (s *Store) SetItemStatus(ctx context.Context, userID, key string, status domain.PlanItemStatus, note string) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO buddy.plan_status (user_id, item_id, status_id, note, updated_at)
		SELECT u.id, i.id, $3, $4, now() FROM buddy.users u JOIN buddy.plan_items i ON i.key = $2
		WHERE u.external_id = $1 AND `+visibleItem+`
		ON CONFLICT (user_id, item_id) DO UPDATE SET status_id = EXCLUDED.status_id, note = EXCLUDED.note,
			updated_at = now()`,
		userID, key, int16(status), note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
