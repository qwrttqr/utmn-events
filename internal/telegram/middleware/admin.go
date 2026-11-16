package middleware

import (
	"context"
	"log/slog"
	"slices"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func RequireAdmin(adminIDs []int64) bot.Middleware {
	adminIDs = slices.Clone(adminIDs)

	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			var userID int64

			switch {
			case update.CallbackQuery != nil:
				userID = update.CallbackQuery.From.ID

			case update.Message != nil && update.Message.From != nil:
				userID = update.Message.From.ID

			default:
				return
			}

			if slices.Contains(adminIDs, userID) {
				next(ctx, b, update)
				return
			}

			slog.WarnContext(ctx, "admin access denied", "user_id", userID)

			if update.CallbackQuery != nil {
				_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
					CallbackQueryID: update.CallbackQuery.ID,
					Text:            "Только для администратора.",
					ShowAlert:       true,
				})
				if err != nil {
					slog.ErrorContext(ctx, "answer callback failed", "error", err)
				}
				return
			}

			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "Только для администратора.",
			})
			if err != nil {
				slog.ErrorContext(ctx, "send access denial failed", "error", err)
			}
		}
	}
}
