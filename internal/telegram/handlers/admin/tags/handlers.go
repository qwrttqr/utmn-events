package tags

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"utmn-events-bot/internal/telegram/navigation"
)

// Register runs before the general admin callback route.
func Register(b *bot.Bot, f *Flow, guard bot.Middleware, beforeBegin func(int64)) {
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:tags", bot.MatchTypeExact,
		func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.CallbackQuery != nil {
				beforeBegin(update.CallbackQuery.From.ID)
			}
			f.handle(ctx, b, update)
		}, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:tag:", bot.MatchTypePrefix, f.handle, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		m := update.Message
		if m == nil || m.From == nil {
			return false
		}
		fields := strings.Fields(m.Text)
		if len(fields) > 0 && (fields[0] == "/start" || fields[0] == "/admin") {
			return false
		}
		return f.enteringName(m.From.ID)
	}, f.handle, guard)
}

func (f *Flow) handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	var userID int64
	var message *models.Message
	var callback, text string
	if q := update.CallbackQuery; q != nil {
		userID, message, callback = q.From.ID, q.Message.Message, q.Data
		if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
			slog.ErrorContext(ctx, "answer tag callback failed", "error", err)
		}
	} else if update.Message != nil && update.Message.From != nil {
		message = update.Message
		userID, text = message.From.ID, message.Text
	}
	if message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	var view screen
	var err error
	if callback == "admin:tags" {
		view, err = f.begin(dbCtx, userID)
	} else {
		view, err = f.process(dbCtx, userID, text, callback)
	}
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag management failed", "user_id", userID, "error", err)
	}
	f.mu.Lock()
	s, exists := f.sessions[userID]
	f.mu.Unlock()
	var snapshot *session
	if exists {
		saved := cloneSession(s)
		snapshot = &saved
	}
	err = navigation.Show(ctx, b, message.Chat.ID, f.page(userID, view, snapshot))
	if err != nil {
		slog.ErrorContext(ctx, "show tag management failed", "error", err)
	}
}
