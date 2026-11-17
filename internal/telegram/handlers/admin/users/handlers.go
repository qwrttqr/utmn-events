package users

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	service     *UsersService
	beforeBegin func(int64)
}

func NewHandler(service *UsersService, beforeBegin func(int64)) *Handler {
	return &Handler{service: service, beforeBegin: beforeBegin}
}
func Register(b *bot.Bot, service *UsersService, guard bot.Middleware, beforeBegin func(int64)) {
	h := NewHandler(service, beforeBegin)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:users", bot.MatchTypeExact, h.List, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:userlist:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "page"))
	}, h.Page, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:userlist:", bot.MatchTypePrefix, h.StaleCallback, guard)
}
func (h *Handler) List(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	h.beforeBegin(q.From.ID)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.list(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "users action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:users:error", Text: "Не удалось загрузить пользователей.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Page(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.page(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "users action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:users:error", Text: "Не удалось загрузить пользователей.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) StaleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.staleCallback(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "users action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:users:error", Text: "Не удалось загрузить пользователей.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}
