package repositories

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"utmn-events-bot/internal/domain"
)

type TagRepository struct {
	pool *pgxpool.Pool
}

func NewTagRepository(pool *pgxpool.Pool) *TagRepository {
	return &TagRepository{pool: pool}
}

// Create returns the existing tag when the normalized name is already present.
func (r *TagRepository) Create(ctx context.Context, name string) (domain.Tag, error) {
	var tag domain.Tag
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tags (name) VALUES ($1)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id, name`, name).Scan(&tag.ID, &tag.Name)
	if err != nil {
		return domain.Tag{}, fmt.Errorf("create tag: %w", err)
	}
	return tag, nil
}

func (r *TagRepository) List(ctx context.Context, limit, offset int) ([]domain.Tag, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name FROM tags ORDER BY name, id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	var tags []domain.Tag
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.Name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read tags: %w", err)
	}
	return tags, nil
}

func (r *TagRepository) ForEvent(ctx context.Context, eventID int64) ([]domain.Tag, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT t.id, t.name FROM tags t
		JOIN tags_to_events te ON te.tag_id = t.id
		WHERE te.event_id = $1 ORDER BY t.name, t.id`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list event tags: %w", err)
	}
	defer rows.Close()
	var tags []domain.Tag
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.Name); err != nil {
			return nil, fmt.Errorf("scan event tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read event tags: %w", err)
	}
	return tags, nil
}

// SetForEvent replaces all assignments in a transaction. Every editor locks the
// same event row, preventing concurrent saves from mixing their selections.
func (r *TagRepository) SetForEvent(ctx context.Context, eventID int64, tagIDs []int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tag assignment: %w", err)
	}
	defer tx.Rollback(ctx)
	var lockedID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM events WHERE id = $1 FOR UPDATE`, eventID).Scan(&lockedID); err != nil {
		return fmt.Errorf("lock event for tags: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tags_to_events WHERE event_id = $1`, eventID); err != nil {
		return fmt.Errorf("clear event tags: %w", err)
	}
	ids := slices.Clone(tagIDs)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range slices.Compact(ids) {
		if _, err := tx.Exec(ctx, `INSERT INTO tags_to_events (event_id, tag_id) VALUES ($1, $2)`, eventID, id); err != nil {
			return fmt.Errorf("assign event tag: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tag assignment: %w", err)
	}
	return nil
}
