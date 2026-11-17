package menu

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct{ beforeOpen func(int64) }

func RegisterCommand(b *bot.Bot, guard bot.Middleware, beforeOpen func(int64)) {
	h := &Handler{beforeOpen: beforeOpen}
	b.RegisterHandler(bot.HandlerTypeMessageText, "admin", bot.MatchTypeCommandStartOnly, h.ShowAdminMenu, guard)
}
func RegisterCallbacks(b *bot.Bot, guard bot.Middleware, beforeOpen func(int64)) {
	h := &Handler{beforeOpen: beforeOpen}
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:menu", bot.MatchTypeExact, h.Menu, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:", bot.MatchTypePrefix, h.Unknown, guard)
}
func (h *Handler) ShowAdminMenu(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	h.beforeOpen(m.From.ID)
	if err := navigation.Show(ctx, b, m.Chat.ID, Page()); err != nil {
		slog.ErrorContext(ctx, "show admin menu failed", "error", err)
	}
}
func (h *Handler) Menu(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	h.beforeOpen(q.From.ID)
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer admin callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, Page()); err != nil {
		slog.ErrorContext(ctx, "show admin page failed", "error", err)
	}
}
func (h *Handler) Unknown(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	h.beforeOpen(q.From.ID)
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer admin callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, unknownPage(q.Data)); err != nil {
		slog.ErrorContext(ctx, "show admin page failed", "error", err)
	}
}
