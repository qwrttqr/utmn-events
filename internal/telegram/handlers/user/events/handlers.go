package events

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
)

func Register(b *bot.Bot, f *Flow, beforeBegin func(int64)) {
	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		q := update.CallbackQuery
		if q == nil || q.Message.Message == nil {
			return
		}
		if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
			slog.ErrorContext(ctx, "answer user event callback failed", "error", err)
		}
		if q.Data == "user:events" || q.Data == "user:registrations" || q.Data == "user:search" {
			beforeBegin(q.From.ID)
		}
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		page, err := f.process(dbCtx, q.From.ID, q.Data)
		cancel()
		if errors.Is(err, domain.ErrEventNotFound) {
			f.Cancel(q.From.ID)
			page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
		} else if err != nil {
			slog.ErrorContext(ctx, "user event action failed", "error", err)
			page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Повторить", CallbackData: q.Data}}, {{Text: "Мероприятия", CallbackData: "user:events"}}}}}
		}
		if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
			slog.ErrorContext(ctx, "show user event page failed", "error", err)
		}
	}
	for _, name := range []string{"user:events", "user:registrations", "user:search"} {
		b.RegisterHandler(bot.HandlerTypeCallbackQueryData, name, bot.MatchTypeExact, handler)
	}
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:event:", bot.MatchTypePrefix, handler)
}
