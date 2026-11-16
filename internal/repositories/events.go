package repositories

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"utmn-events-bot/internal/domain"
)

type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

// Browse matches any selected tag, without duplicating events that have several
// matching tags. UserTelegramID filters to that user's registrations.
func (r *EventRepository) Browse(ctx context.Context, filter domain.EventFilter, limit, offset int) ([]domain.EventSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.title FROM events e
		WHERE (NOT $1::boolean OR e.starts_at > clock_timestamp())
		AND (COALESCE(cardinality($2::bigint[]), 0) = 0 OR EXISTS (
		    SELECT 1 FROM tags_to_events te WHERE te.event_id = e.id AND te.tag_id = ANY($2::bigint[])))
		AND ($3::bigint = 0 OR EXISTS (
		    SELECT 1 FROM event_registrations er JOIN users u ON u.id = er.user_id
		    WHERE er.event_id = e.id AND u.tg_id = $3))
		ORDER BY e.starts_at ASC NULLS LAST, e.id ASC LIMIT $4 OFFSET $5`,
		filter.UpcomingOnly, filter.TagIDs, filter.UserTelegramID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("browse events: %w", err)
	}
	defer rows.Close()
	var events []domain.EventSummary
	for rows.Next() {
		var event domain.EventSummary
		if err := rows.Scan(&event.ID, &event.Title); err != nil {
			return nil, fmt.Errorf("scan browsed event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read browsed events: %w", err)
	}
	return events, nil
}

func (r *EventRepository) List(ctx context.Context, limit, offset int) ([]domain.EventSummary, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, title FROM events ORDER BY starts_at DESC NULLS LAST, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	var events []domain.EventSummary
	for rows.Next() {
		var event domain.EventSummary
		if err := rows.Scan(&event.ID, &event.Title); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}
	return events, nil
}

func (r *EventRepository) Create(ctx context.Context, event domain.Event) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin event creation: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO events (title, description, photo_file_id, location, starts_at, ends_at, capacity)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, $7)
		RETURNING id`, event.Title, event.Description, event.PhotoFileID,
		event.Location, event.StartsAt.UTC(), event.EndsAt.UTC(), event.Capacity,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	tags := slices.Clone(event.Tags)
	sort.Strings(tags) // Keep lock acquisition order consistent across creators.
	for _, name := range slices.Compact(tags) {
		// Separate statements also handle a concurrent insert of the same tag.
		if _, err := tx.Exec(ctx, `INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, name); err != nil {
			return 0, fmt.Errorf("insert event tag: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tags_to_events (event_id, tag_id)
			SELECT $1, id FROM tags WHERE name = $2`, id, name); err != nil {
			return 0, fmt.Errorf("link event tag: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit event creation: %w", err)
	}
	return id, nil
}

func (r *EventRepository) Get(ctx context.Context, id int64) (domain.Event, error) {
	var event domain.Event
	var start, end *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT e.id, e.title, COALESCE(e.description, ''), COALESCE(e.photo_file_id, ''),
		       e.location, e.starts_at, e.ends_at, e.capacity,
		       ARRAY(SELECT DISTINCT t.name FROM tags t JOIN tags_to_events te ON te.tag_id = t.id WHERE te.event_id = e.id ORDER BY t.name),
		       (SELECT count(*) FROM event_registrations er WHERE er.event_id = e.id)
		FROM events e WHERE e.id = $1`, id).Scan(&event.ID, &event.Title, &event.Description, &event.PhotoFileID,
		&event.Location, &start, &end, &event.Capacity, &event.Tags, &event.RegistrationCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Event{}, domain.ErrEventNotFound
	}
	if err != nil {
		return domain.Event{}, fmt.Errorf("get event: %w", err)
	}
	if start != nil {
		event.StartsAt = start.UTC()
	}
	if end != nil {
		event.EndsAt = end.UTC()
	}
	return event, nil
}

func (r *EventRepository) Update(ctx context.Context, event domain.Event, field domain.EventField) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin event update: %w", err)
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM events WHERE id = $1 FOR UPDATE`, event.ID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrEventNotFound
	}
	if err != nil {
		return fmt.Errorf("lock event update: %w", err)
	}
	var query string
	var args []any
	switch field {
	case domain.EventTitle:
		query, args = `UPDATE events SET title = $2 WHERE id = $1`, []any{id, event.Title}
	case domain.EventDescription:
		query, args = `UPDATE events SET description = NULLIF($2, '') WHERE id = $1`, []any{id, event.Description}
	case domain.EventPhoto:
		query, args = `UPDATE events SET photo_file_id = NULLIF($2, '') WHERE id = $1`, []any{id, event.PhotoFileID}
	case domain.EventLocation:
		query, args = `UPDATE events SET location = $2 WHERE id = $1`, []any{id, event.Location}
	case domain.EventDates:
		if event.StartsAt.IsZero() || !event.EndsAt.After(event.StartsAt) {
			return errors.New("invalid event date range")
		}
		query, args = `UPDATE events SET starts_at = $2, ends_at = $3 WHERE id = $1`, []any{id, event.StartsAt.UTC(), event.EndsAt.UTC()}
	case domain.EventCapacity:
		if event.Capacity != nil {
			var count int64
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_registrations WHERE event_id = $1`, id).Scan(&count); err != nil {
				return fmt.Errorf("count event registrations: %w", err)
			}
			if int64(*event.Capacity) < count {
				return domain.ErrCapacityBelowRegistrations
			}
		}
		query, args = `UPDATE events SET capacity = $2 WHERE id = $1`, []any{id, event.Capacity}
	case domain.EventTags:
		if _, err := tx.Exec(ctx, `DELETE FROM tags_to_events WHERE event_id = $1`, id); err != nil {
			return fmt.Errorf("clear updated tags: %w", err)
		}
		names := slices.Clone(event.Tags)
		sort.Strings(names)
		for _, name := range slices.Compact(names) {
			if _, err := tx.Exec(ctx, `INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, name); err != nil {
				return fmt.Errorf("create updated tag: %w", err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO tags_to_events (event_id, tag_id) SELECT $1, id FROM tags WHERE name = $2`, id, name); err != nil {
				return fmt.Errorf("assign updated tag: %w", err)
			}
		}
	default:
		return errors.New("unsupported event field")
	}
	if query != "" {
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("update event field: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit event update: %w", err)
	}
	return nil
}

func (r *EventRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin event deletion: %w", err)
	}
	defer tx.Rollback(ctx)
	var lockedID int64
	err = tx.QueryRow(ctx, `SELECT id FROM events WHERE id = $1 FOR UPDATE`, id).Scan(&lockedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrEventNotFound
	}
	if err != nil {
		return fmt.Errorf("lock event deletion: %w", err)
	}
	for _, query := range []string{
		`DELETE FROM event_registrations WHERE event_id = $1`,
		`DELETE FROM tags_to_events WHERE event_id = $1`,
		`DELETE FROM events WHERE id = $1`,
	} {
		if _, err := tx.Exec(ctx, query, id); err != nil {
			return fmt.Errorf("delete event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit event deletion: %w", err)
	}
	return nil
}
