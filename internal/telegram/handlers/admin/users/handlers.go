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
	service *UsersService
}

func NewHandler(service *UsersService) *Handler {
	return &Handler{service: service}
}
func Register(b *bot.Bot, service *UsersService, guard bot.Middleware) {
	h := NewHandler(service)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:users", bot.MatchTypeExact, h.List, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		action, ok := strings.CutPrefix(q.Data, paginationPrefix)
		return ok && strings.HasPrefix(action, "page") && !strings.Contains(action, ":")
	}, h.Page, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, paginationPrefix, bot.MatchTypePrefix, h.StaleCallback, guard)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.list(dbCtx, q.From.ID)
	cancel()
	page := h.page(q.From.ID, result)
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
	result, err := h.service.page(dbCtx, q.From.ID, strings.TrimPrefix(q.Data, paginationPrefix))
	cancel()
	page := h.page(q.From.ID, result)
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

	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, expiredPage()); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}
