package users

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/pagination"
	"utmn-events-bot/internal/telegram/presentation"
)

type Repository interface {
	List(context.Context, int, int) ([]domain.User, error)
}
type session struct {
	page  int
	token string
}
type Flow struct {
	mu       sync.Mutex
	sessions map[int64]session
	repo     Repository
	adminIDs []int64
}

func New(repo Repository, adminIDs []int64) *Flow {
	return &Flow{sessions: make(map[int64]session), repo: repo, adminIDs: slices.Clone(adminIDs)}
}
func (f *Flow) Cancel(id int64) { f.mu.Lock(); defer f.mu.Unlock(); delete(f.sessions, id) }

func Register(b *bot.Bot, f *Flow, guard bot.Middleware, beforeBegin func(int64)) {
	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		q := update.CallbackQuery
		if q == nil || q.Message.Message == nil {
			return
		}
		if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
			slog.ErrorContext(ctx, "answer user list callback failed", "error", err)
		}
		if q.Data == "admin:users" {
			beforeBegin(q.From.ID)
		}
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		page, err := f.process(dbCtx, q.From.ID, q.Data)
		cancel()
		if err != nil {
			slog.ErrorContext(ctx, "list users failed", "error", err)
			page = navigation.Page{Key: "admin:users:error", Text: "Не удалось загрузить пользователей.", Admin: true, Keyboard: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Повторить", CallbackData: "admin:users"}}}}}
		}
		if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
			slog.ErrorContext(ctx, "show user list failed", "error", err)
		}
	}
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:users", bot.MatchTypeExact, handler, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:userlist:", bot.MatchTypePrefix, handler, guard)
}

func (f *Flow) process(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists := f.sessions[userID]
	if callback == "admin:users" {
		s = session{}
	} else {
		parts := strings.Split(callback, ":")
		if !exists || len(parts) != 4 || parts[2] != s.token {
			return navigation.Page{Key: "admin:users:expired", Text: "Эта кнопка устарела. Откройте список пользователей заново.", Admin: true}, nil
		}
		if page, ok := pagination.Move(s.page, parts[3]); ok {
			s.page = page
		}
	}
	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err == nil {
		f.sessions[userID] = s
	}
	return page, err
}

func (f *Flow) render(ctx context.Context, userID int64, s session) (navigation.Page, error) {
	users, err := f.repo.List(ctx, pagination.Size+1, s.page*pagination.Size)
	if err != nil {
		return navigation.Page{}, err
	}
	more := len(users) > pagination.Size
	if more {
		users = users[:pagination.Size]
	}
	markup := &models.InlineKeyboardMarkup{}
	pagination.Add(markup, s.page, more, func(action string) string { return "admin:userlist:" + s.token + ":" + action })
	page := navigation.Page{Key: "admin:users", Text: fmt.Sprintf("Все пользователи\nСтраница %d\n\n%s", s.page+1, presentation.Users(users, s.page*pagination.Size, f.adminIDs)), Admin: true, Keyboard: markup}
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		s.token = rand.Text()
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		page, err := f.render(dbCtx, userID, s)
		if err == nil {
			f.sessions[userID] = s
		}
		return page, err
	}
	return page, nil
}
