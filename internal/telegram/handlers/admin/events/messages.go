package events

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/navigation"
)

func answerEventCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer event callback failed", "error", err)
	}
}

func (f *Flow) show(ctx context.Context, b *bot.Bot, userID, chatID int64, view eventScreen) {
	f.mu.Lock()
	d, exists := f.drafts[userID]
	f.mu.Unlock()
	var snapshot *eventDraft
	if exists {
		cloned := cloneDraft(d)
		snapshot = &cloned
	}
	if err := navigation.Show(ctx, b, chatID, f.page(userID, view, snapshot)); err != nil {
		slog.ErrorContext(ctx, "show event screen failed", "error", err)
	}
}
