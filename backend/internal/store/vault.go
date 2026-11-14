package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
)

const vaultColumns = `v.id, u.external_id, v.title, v.summary, v.tags, v.sources, v.version, v.created_at, v.updated_at`

func scanVault(r pgx.Row) (domain.VaultItem, error) {
	var v domain.VaultItem
	err := r.Scan(&v.ID, &v.UserID, &v.Title, &v.Summary, &v.Tags, &v.Sources, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.VaultItem{}, ErrNotFound
	}
	return v, err
}

func (s *Store) CreateVaultItem(ctx context.Context, v domain.VaultItem) (domain.VaultItem, error) {
	var out domain.VaultItem
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = scanVault(tx.QueryRow(ctx, `
			WITH v AS (
				INSERT INTO buddy.vault_items (user_id, title, summary, tags, sources)
				SELECT id, $2, $3, $4, $5 FROM buddy.users WHERE external_id = $1
				RETURNING *
			)
			SELECT `+vaultColumns+` FROM v JOIN buddy.users u ON u.id = v.user_id`,
			v.UserID, v.Title, v.Summary, orStrings(v.Tags), orArray(v.Sources)))
		if err != nil {
			return err
		}
		return saveVersion(ctx, tx, out)
	})
	return out, err
}

func saveVersion(ctx context.Context, tx pgx.Tx, v domain.VaultItem) error {
	_, err := tx.Exec(ctx, `INSERT INTO buddy.vault_versions (item_id, version, title, summary, tags, sources)
		VALUES ($1, $2, $3, $4, $5, $6)`, v.ID, v.Version, v.Title, v.Summary, orStrings(v.Tags), orArray(v.Sources))
	return err
}

// VaultItems lists the user's records, last changed first; a tag narrows the list.
func (s *Store) VaultItems(ctx context.Context, userID, tag string) ([]domain.VaultItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+vaultColumns+`
		FROM buddy.vault_items v JOIN buddy.users u ON u.id = v.user_id
		WHERE u.external_id = $1 AND ($2 = '' OR $2 = ANY(v.tags))
		ORDER BY v.updated_at DESC, v.id`, userID, tag)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.VaultItem, error) { return scanVault(r) })
}

// VaultItem returns the user's record with its versions, newest first; someone else's is ErrNotFound.
func (s *Store) VaultItem(ctx context.Context, userID, id string) (domain.VaultItem, error) {
	if uuid.Validate(id) != nil {
		return domain.VaultItem{}, ErrNotFound
	}
	v, err := scanVault(s.pool.QueryRow(ctx, `SELECT `+vaultColumns+`
		FROM buddy.vault_items v JOIN buddy.users u ON u.id = v.user_id
		WHERE v.id = $1 AND u.external_id = $2`, id, userID))
	if err != nil {
		return domain.VaultItem{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT version, title, summary, tags, sources, created_at
		FROM buddy.vault_versions WHERE item_id = $1 ORDER BY version DESC`, id)
	if err != nil {
		return domain.VaultItem{}, err
	}
	v.Versions, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.VaultItem, error) {
		ver := domain.VaultItem{ID: v.ID, UserID: v.UserID}
		err := r.Scan(&ver.Version, &ver.Title, &ver.Summary, &ver.Tags, &ver.Sources, &ver.CreatedAt)
		return ver, err
	})
	return v, err
}

// UpdateVaultItem applies a patch as a new version.
func (s *Store) UpdateVaultItem(ctx context.Context, userID, id string, p domain.VaultPatch) (domain.VaultItem, error) {
	cur, err := s.VaultItem(ctx, userID, id)
	if err != nil {
		return domain.VaultItem{}, err
	}
	next := cur
	if p.Title != nil {
		next.Title = *p.Title
	}
	if p.Summary != nil {
		next.Summary = *p.Summary
	}
	if p.Tags != nil {
		next.Tags = p.Tags
	}
	return s.writeVersion(ctx, next)
}

// RestoreVaultVersion makes an old version current again, as a new version.
func (s *Store) RestoreVaultVersion(ctx context.Context, userID, id string, version int) (domain.VaultItem, error) {
	cur, err := s.VaultItem(ctx, userID, id)
	if err != nil {
		return domain.VaultItem{}, err
	}
	for _, old := range cur.Versions {
		if old.Version == version {
			next := cur
			next.Title, next.Summary, next.Tags, next.Sources = old.Title, old.Summary, old.Tags, old.Sources
			return s.writeVersion(ctx, next)
		}
	}
	return domain.VaultItem{}, ErrNotFound
}

func (s *Store) writeVersion(ctx context.Context, v domain.VaultItem) (domain.VaultItem, error) {
	var out domain.VaultItem
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = scanVault(tx.QueryRow(ctx, `
			WITH v AS (
				UPDATE buddy.vault_items SET title = $2, summary = $3, tags = $4, sources = $5,
					version = version + 1, updated_at = now()
				WHERE id = $1 RETURNING *
			)
			SELECT `+vaultColumns+` FROM v JOIN buddy.users u ON u.id = v.user_id`,
			v.ID, v.Title, v.Summary, orStrings(v.Tags), orArray(v.Sources)))
		if err != nil {
			return err
		}
		return saveVersion(ctx, tx, out)
	})
	return out, err
}

func (s *Store) DeleteVaultItem(ctx context.Context, userID, id string) error {
	if uuid.Validate(id) != nil {
		return ErrNotFound
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM buddy.vault_items
		WHERE id = $2 AND user_id = (SELECT id FROM buddy.users WHERE external_id = $1)`, userID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func orStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
