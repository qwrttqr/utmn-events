package repositories

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"

	"github.com/jackc/pgx/v5/pgxpool"

	"utmn-events-bot/internal/domain"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) IsRegistered(ctx context.Context, telegramID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE tg_id = $1)`, telegramID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check user registration: %w", err)
	}
	return exists, nil
}

func (r *UserRepository) Create(ctx context.Context, user domain.User) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (tg_id, tg_username, firstname, lastname, surname)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		ON CONFLICT (tg_id) DO NOTHING`,
		user.TelegramID, user.Username, user.FirstName, user.LastName, user.Surname,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *UserRepository) List(ctx context.Context, limit, offset int) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, tg_id, COALESCE(tg_username, ''), firstname, lastname, COALESCE(surname, ''), role FROM users ORDER BY id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	return readUsers(rows)
}

func (r *UserRepository) ListForEvent(ctx context.Context, eventID int64, limit, offset int) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT u.id, u.tg_id, COALESCE(u.tg_username, ''), u.firstname, u.lastname, COALESCE(u.surname, ''), u.role
		FROM users u JOIN event_registrations er ON er.user_id = u.id
		WHERE er.event_id = $1 ORDER BY er.id LIMIT $2 OFFSET $3`, eventID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list event participants: %w", err)
	}
	defer rows.Close()
	return readUsers(rows)
}

func readUsers(rows pgx.Rows) ([]domain.User, error) {
	var users []domain.User
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.TelegramID, &user.Username, &user.FirstName, &user.LastName, &user.Surname, &user.Role); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}
	return users, nil
}
