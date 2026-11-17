package users

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/pagination"
)

type Repository interface {
	List(context.Context, int, int) ([]domain.User, error)
}
type conversation struct {
	page  int
	token string
}
type UsersService struct {
	mu       sync.Mutex
	machine  *fsm.FSM[string, conversation]
	repo     Repository
	adminIDs []int64
}

func NewService(repo Repository, adminIDs []int64) *UsersService {
	return &UsersService{machine: fsm.NewInMemoryFSM[string, conversation]("idle"), repo: repo, adminIDs: slices.Clone(adminIDs)}
}
func (f *UsersService) Cancel(id int64) { f.mu.Lock(); defer f.mu.Unlock(); f.clear(id) }

func (f *UsersService) process(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	if callback == "admin:users" {
		return f.list(ctx, userID)
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 {
		return f.staleCallback(ctx, userID, callback)
	}
	action := parts[3]
	if strings.HasPrefix(action, "page") {
		return f.page(ctx, userID, callback)
	}
	return f.staleCallback(ctx, userID, callback)
}

func (f *UsersService) page(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:users:expired", Text: "Эта кнопка устарела. Откройте список пользователей заново.", Admin: true}, nil
	}
	if page, ok := pagination.Move(s.page, parts[3]); ok {
		s.page = page
	}
	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err == nil {
		if err := f.save(ctx, userID, s); err != nil {
			return navigation.Page{}, err
		}
	}
	return page, err
}

func (f *UsersService) staleCallback(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:users:expired", Text: "Эта кнопка устарела. Откройте список пользователей заново.", Admin: true}, nil
	}
	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err == nil {
		if err := f.save(ctx, userID, s); err != nil {
			return navigation.Page{}, err
		}
	}
	return page, err
}

func (f *UsersService) list(ctx context.Context, userID int64) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	s := conversation{}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err == nil {
		if err := f.save(ctx, userID, s); err != nil {
			return navigation.Page{}, err
		}
	}
	return page, err
}

func (f *UsersService) render(ctx context.Context, userID int64, s conversation) (navigation.Page, error) {
	users, err := f.repo.List(ctx, pagination.Size+1, s.page*pagination.Size)
	if err != nil {
		return navigation.Page{}, err
	}
	more := len(users) > pagination.Size
	if more {
		users = users[:pagination.Size]
	}

	page := navigation.Page{Key: "admin:users", Text: fmt.Sprintf("Все пользователи\n\n%s", formatUsers(users, s.page*pagination.Size, f.adminIDs)), Admin: true, Keyboard: usersKeyboard(s, more)}
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		s.token = rand.Text()
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		page, err := f.render(dbCtx, userID, s)
		if err == nil {
			if err := f.save(ctx, userID, s); err != nil {
				return navigation.Page{}, err
			}
		}
		return page, err
	}
	return page, nil
}

const usersList fsm.StateID = "admin.users.list"

func reportStorageError(err error) { slog.Error("conversation storage failed", "error", err) }
func (f *UsersService) load(ctx context.Context, userID int64) (conversation, bool, error) {
	state, err := f.machine.CurrentState(ctx, userID)
	if err != nil {
		return conversation{}, false, err
	}
	value, err := f.machine.GetData(ctx, userID, "session")
	if errors.Is(err, fsm.ErrUserDataNotFound) {
		return conversation{}, false, nil
	}
	if err != nil {
		return conversation{}, false, err
	}
	if state == usersList {
		return value, true, nil
	}
	return conversation{}, false, fmt.Errorf("unexpected conversation state %q", state)
}
func (f *UsersService) save(ctx context.Context, userID int64, value conversation) error {
	if err := f.machine.SetData(ctx, userID, "session", value); err != nil {
		return err
	}
	return f.machine.Transition(ctx, userID, usersList)
}
func (f *UsersService) clear(userID int64) {
	if err := f.machine.Finish(context.Background(), userID); err != nil {
		reportStorageError(err)
	}
}
