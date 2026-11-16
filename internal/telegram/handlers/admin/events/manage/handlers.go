package manage

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

func Register(b *bot.Bot, f *Flow, guard bot.Middleware, beforeBegin func(int64)) {
	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) { f.handle(ctx, b, update, beforeBegin) }
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:events", bot.MatchTypeExact, handler, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:manage:", bot.MatchTypePrefix, handler, guard)
}

func (f *Flow) handle(ctx context.Context, b *bot.Bot, update *models.Update, beforeBegin func(int64)) {
	q := update.CallbackQuery
	if q == nil || q.Message.Message == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer event management callback failed", "error", err)
	}
	if q.Data == "admin:events" {
		beforeBegin(q.From.ID)
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, edit, err := f.process(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		f.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event management failed", "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Повторить", CallbackData: q.Data}}, {{Text: "Мероприятия", CallbackData: "admin:events"}}}}}
	}
	if edit != nil {
		beforeBegin(q.From.ID)
		if err := f.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show event management failed", "error", err)
	}
}
