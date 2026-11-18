package menu

import (
	"context"
	"log/slog"
	"slices"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	navigator  *navigation.Navigator
	adminIDs   []int64
	beforeOpen func(int64)
}

func Register(b *bot.Bot, navigator *navigation.Navigator, adminIDs []int64, beforeOpen func(int64)) {
	h := &Handler{navigator: navigator, adminIDs: slices.Clone(adminIDs), beforeOpen: beforeOpen}
	b.RegisterHandler(bot.HandlerTypeMessageText, "start", bot.MatchTypeCommandStartOnly, h.Start)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:menu", bot.MatchTypeExact, h.Menu)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "home:menu", bot.MatchTypeExact, h.Home)
}
func (h *Handler) Start(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	h.beforeOpen(m.From.ID)
	if err := h.navigator.Show(ctx, b, update, m.Chat.ID, DefaultPage(m.From.ID, h.adminIDs)); err != nil {
		slog.ErrorContext(ctx, "show start menu failed", "error", err)
	}
}
func (h *Handler) Menu(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer menu callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	h.beforeOpen(q.From.ID)
	if err := h.navigator.Show(ctx, b, update, q.Message.Message.Chat.ID, GetMainPage(q.From.ID, h.adminIDs)); err != nil {
		slog.ErrorContext(ctx, "show user menu failed", "error", err)
	}
}
func (h *Handler) Home(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer menu callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	h.beforeOpen(q.From.ID)
	if err := h.navigator.Show(ctx, b, update, q.Message.Message.Chat.ID, DefaultPage(q.From.ID, h.adminIDs)); err != nil {
		slog.ErrorContext(ctx, "show user menu failed", "error", err)
	}
}
