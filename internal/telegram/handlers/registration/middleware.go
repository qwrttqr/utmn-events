package registration

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (p *Pipeline) RequireRegistered(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var user *models.User
		var message *models.Message
		var callback string
		if update.CallbackQuery != nil {
			user = &update.CallbackQuery.From
			message = update.CallbackQuery.Message.Message
			callback = update.CallbackQuery.Data
		} else if update.Message != nil {
			message = update.Message
			user = message.From
		}
		if user == nil {
			return
		}

		if message == nil || message.Chat.Type != models.ChatTypePrivate || message.Chat.ID != user.ID {
			if update.CallbackQuery != nil {
				answer(ctx, b, update.CallbackQuery.ID, "Откройте личный чат с ботом и отправьте /start.")
			} else if message != nil {
				send(ctx, b, message.Chat.ID, screen{text: "Откройте личный чат с ботом и отправьте /start."})
			}
			return
		}

		text := message.Text
		if update.CallbackQuery != nil {
			text = "" // The bot's previous prompt is not user input.
		}
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		view, handled, err := p.process(dbCtx, user.ID, user.Username, text, callback)
		cancel()
		if err != nil {
			slog.ErrorContext(ctx, "registration failed", "user_id", user.ID, "error", err)
			view.text = "Не удалось обратиться к базе данных. Попробуйте ещё раз.\n\n" + view.text
			view.temporary = true
		}
		if !handled {
			next(ctx, b, update)
			return
		}
		if update.CallbackQuery != nil {
			answer(ctx, b, update.CallbackQuery.ID, "")
		}
		send(ctx, b, message.Chat.ID, view)
	}
}
