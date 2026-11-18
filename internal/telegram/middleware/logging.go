package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func LogUpdateDuration(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		start := time.Now()
		defer func() {
			kind := "message"
			if update.CallbackQuery != nil {
				kind = "callback_query"
			}
			slog.InfoContext(ctx, "telegram update handled", "update_id", update.ID,
				"kind", kind, "duration_ms", float64(time.Since(start))/float64(time.Millisecond))
		}()
		next(ctx, b, update)
	}
}
