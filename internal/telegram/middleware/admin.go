package middleware

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type AdminChecker func(context.Context, int64) bool

func NewAdminChecker(configuredIDs []int64, registrationStatus func(context.Context, int64) (bool, bool, error)) AdminChecker {
	configuredIDs = slices.Clone(configuredIDs)
	return func(ctx context.Context, userID int64) bool {
		if slices.Contains(configuredIDs, userID) {
			return true
		}
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		registered, admin, err := registrationStatus(dbCtx, userID)
		if err != nil {
			slog.ErrorContext(ctx, "check admin role failed", "user_id", userID, "error", err)
			return false
		}
		return registered && admin
	}
}

func RequireAdmin(isAdmin AdminChecker) bot.Middleware {
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

			if isAdmin(ctx, userID) {
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
