package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"utmn-events-bot/internal/domain"
)

type registrationDB interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type EventRegistrationRepository struct{ db registrationDB }

func NewEventRegistrationRepository(pool *pgxpool.Pool) *EventRegistrationRepository {
	return &EventRegistrationRepository{db: pool}
}

func (r *EventRegistrationRepository) IsEnrolled(ctx context.Context, eventID, telegramID int64) (bool, error) {
	var enrolled bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM event_registrations er JOIN users u ON u.id = er.user_id
		WHERE er.event_id = $1 AND u.tg_id = $2)`, eventID, telegramID).Scan(&enrolled)
	if err != nil {
		return false, fmt.Errorf("check event enrollment: %w", err)
	}
	return enrolled, nil
}

func (r *EventRegistrationRepository) Enroll(ctx context.Context, eventID, telegramID int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin enrollment: %w", err)
	}
	defer tx.Rollback(ctx)
	var startsAt *time.Time
	var capacity *int32
	err = tx.QueryRow(ctx, `SELECT starts_at, capacity FROM events WHERE id = $1 FOR UPDATE`, eventID).Scan(&startsAt, &capacity)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrEventNotFound
	}
	if err != nil {
		return fmt.Errorf("lock event enrollment: %w", err)
	}
	var userID int64
	err = tx.QueryRow(ctx, `SELECT id FROM users WHERE tg_id = $1`, telegramID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrUserNotRegistered
	}
	if err != nil {
		return fmt.Errorf("find enrollment user: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM event_registrations WHERE event_id = $1 AND user_id = $2)`, eventID, userID).Scan(&exists); err != nil {
		return fmt.Errorf("check duplicate enrollment: %w", err)
	}
	if exists {
		return nil
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return fmt.Errorf("read enrollment clock: %w", err)
	}
	if startsAt == nil || !startsAt.After(now) {
		return domain.ErrEventClosed
	}
	if capacity != nil {
		var count int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_registrations WHERE event_id = $1`, eventID).Scan(&count); err != nil {
			return fmt.Errorf("count enrollments: %w", err)
		}
		if count >= int64(*capacity) {
			return domain.ErrEventFull
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO event_registrations (event_id, user_id) VALUES ($1, $2) ON CONFLICT (event_id, user_id) DO NOTHING`, eventID, userID); err != nil {
		return fmt.Errorf("insert enrollment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit enrollment: %w", err)
	}
	return nil
}

func (r *EventRegistrationRepository) Unenroll(ctx context.Context, eventID, telegramID int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin unenrollment: %w", err)
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `SELECT id FROM events WHERE id = $1 FOR UPDATE`, eventID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrEventNotFound
	}
	if err != nil {
		return fmt.Errorf("lock event unenrollment: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM event_registrations er USING users u WHERE er.user_id = u.id AND er.event_id = $1 AND u.tg_id = $2`, eventID, telegramID); err != nil {
		return fmt.Errorf("delete enrollment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit unenrollment: %w", err)
	}
	return nil
}
