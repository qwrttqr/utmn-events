package repositories

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"utmn-events-bot/internal/domain"
)

type rowMock func(...any) error

func (r rowMock) Scan(dest ...any) error { return r(dest...) }

type enrollmentDBMock struct {
	mu            sync.Mutex
	users         map[int64]int64
	registrations map[int64]bool
	capacity      int32
	now, startsAt time.Time
	missing       bool
	loseCommitAck bool
}

func newEnrollmentDBMock() *enrollmentDBMock {
	return &enrollmentDBMock{users: map[int64]int64{111: 1, 222: 2}, registrations: make(map[int64]bool), capacity: 1,
		now: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), startsAt: time.Date(2031, 1, 1, 0, 0, 0, 0, time.UTC)}
}
func (d *enrollmentDBMock) Begin(context.Context) (pgx.Tx, error) {
	return &enrollmentTxMock{db: d}, nil
}
func (d *enrollmentDBMock) QueryRow(context.Context, string, ...any) pgx.Row {
	return rowMock(func(...any) error { return errors.New("unexpected pool query") })
}

type enrollmentTxMock struct {
	pgx.Tx
	db             *enrollmentDBMock
	locked         bool
	insert, remove int64
}

func (tx *enrollmentTxMock) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	return rowMock(func(dest ...any) error {
		if strings.Contains(query, "FROM events") {
			if !strings.Contains(query, "FOR UPDATE") {
				return errors.New("event must be locked")
			}
			tx.db.mu.Lock()
			tx.locked = true
			if tx.db.missing {
				return pgx.ErrNoRows
			}
			if strings.Contains(query, "starts_at") {
				start, capacity := tx.db.startsAt, tx.db.capacity
				*dest[0].(**time.Time), *dest[1].(**int32) = &start, &capacity
			} else {
				*dest[0].(*int64) = 42
			}
			return nil
		}
		if !tx.locked {
			return errors.New("query executed before event lock")
		}
		switch {
		case strings.Contains(query, "FROM users"):
			id, ok := tx.db.users[args[0].(int64)]
			if !ok {
				return pgx.ErrNoRows
			}
			*dest[0].(*int64) = id
		case strings.Contains(query, "SELECT EXISTS"):
			*dest[0].(*bool) = tx.db.registrations[args[1].(int64)]
		case strings.Contains(query, "clock_timestamp"):
			*dest[0].(*time.Time) = tx.db.now
		case strings.Contains(query, "count(*)"):
			*dest[0].(*int64) = int64(len(tx.db.registrations))
		default:
			return errors.New("unexpected transaction query")
		}
		return nil
	})
}
func (tx *enrollmentTxMock) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if !tx.locked {
		return pgconn.CommandTag{}, errors.New("write executed before event lock")
	}
	if strings.HasPrefix(query, "INSERT") {
		tx.insert = args[1].(int64)
	} else if strings.HasPrefix(query, "DELETE") {
		tx.remove = tx.db.users[args[1].(int64)]
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (tx *enrollmentTxMock) Commit(context.Context) error {
	if tx.insert != 0 {
		tx.db.registrations[tx.insert] = true
	}
	if tx.remove != 0 {
		delete(tx.db.registrations, tx.remove)
	}
	fail := tx.db.loseCommitAck
	tx.db.loseCommitAck = false
	tx.unlock()
	if fail {
		return errors.New("commit acknowledgement lost")
	}
	return nil
}
func (tx *enrollmentTxMock) Rollback(context.Context) error { tx.unlock(); return nil }
func (tx *enrollmentTxMock) unlock() {
	if tx.locked {
		tx.locked = false
		tx.db.mu.Unlock()
	}
}

func TestConcurrentEnrollmentForLastSeat(t *testing.T) {
	db := newEnrollmentDBMock()
	repo := &EventRegistrationRepository{db: db}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, telegramID := range []int64{111, 222} {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); results <- repo.Enroll(context.Background(), 42, id) }(telegramID)
	}
	wg.Wait()
	close(results)
	success, full := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, domain.ErrEventFull) {
			full++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || full != 1 || len(db.registrations) != 1 {
		t.Fatal("last seat was not assigned exactly once")
	}
}

func TestEnrollmentRetryAfterLostCommitAcknowledgement(t *testing.T) {
	db := newEnrollmentDBMock()
	db.loseCommitAck = true
	repo := &EventRegistrationRepository{db: db}
	if err := repo.Enroll(context.Background(), 42, 111); err == nil {
		t.Fatal("expected commit error")
	}
	if err := repo.Enroll(context.Background(), 42, 111); err != nil {
		t.Fatal(err)
	}
	if len(db.registrations) != 1 {
		t.Fatal("retry duplicated enrollment")
	}
}

func TestUnenrollmentOnlyRemovesTheActorAndIsIdempotent(t *testing.T) {
	db := newEnrollmentDBMock()
	db.registrations[1], db.registrations[2] = true, true
	repo := &EventRegistrationRepository{db: db}
	for i := 0; i < 2; i++ {
		if err := repo.Unenroll(context.Background(), 42, 111); err != nil {
			t.Fatal(err)
		}
	}
	if db.registrations[1] || !db.registrations[2] {
		t.Fatal("unenrollment affected another user's registration")
	}
}

func TestEnrollmentRejectsPastMissingAndUnregistered(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*enrollmentDBMock)
		actor     int64
		want      error
	}{
		{"past", func(d *enrollmentDBMock) { d.startsAt = d.now.Add(-time.Hour) }, 111, domain.ErrEventClosed},
		{"missing", func(d *enrollmentDBMock) { d.missing = true }, 111, domain.ErrEventNotFound},
		{"unknown user", func(*enrollmentDBMock) {}, 333, domain.ErrUserNotRegistered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newEnrollmentDBMock()
			tc.configure(db)
			if err := (&EventRegistrationRepository{db: db}).Enroll(context.Background(), 42, tc.actor); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if len(db.registrations) != 0 {
				t.Fatal("invalid enrollment was inserted")
			}
		})
	}
}
