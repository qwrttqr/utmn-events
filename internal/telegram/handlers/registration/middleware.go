package registration

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h *Handler) RequireRegistered(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var user *models.User
		var message *models.Message
		if q := update.CallbackQuery; q != nil {
			user, message = &q.From, q.Message.Message
		} else if m := update.Message; m != nil {
			user, message = m.From, m
		}
		if user == nil {
			return
		}
		if message == nil || message.Chat.Type != models.ChatTypePrivate || message.Chat.ID != user.ID {
			if q := update.CallbackQuery; q != nil {
				answer(ctx, b, q.ID, "Откройте личный чат с ботом и отправьте /start.")
			} else if message != nil {
				h.send(ctx, b, update, message.Chat.ID, registrationPage("Откройте личный чат с ботом и отправьте /start."))
			}
			return
		}

		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		registered, _, err := h.service.users.RegistrationStatus(dbCtx, user.ID)
		cancel()
		if err != nil {
			slog.ErrorContext(ctx, "check registration failed", "user_id", user.ID, "error", err)
			if q := update.CallbackQuery; q != nil {
				answer(ctx, b, q.ID, "")
			}
			page := registrationPage("Не удалось проверить регистрацию. Попробуйте ещё раз.")
			page.Root = false
			h.send(ctx, b, update, message.Chat.ID, page)
			return
		}
		if registered {
			h.service.Cancel(user.ID)
			if q := update.CallbackQuery; q != nil && strings.HasPrefix(q.Data, "registration:") {
				answer(ctx, b, q.ID, "")
				h.showHome(ctx, b, update, message.Chat.ID)
				return
			}
			next(ctx, b, update)
			return
		}

		if q := update.CallbackQuery; q != nil {
			if strings.HasPrefix(q.Data, "registration:") {
				next(ctx, b, update)
			} else {
				h.Preview(ctx, b, update)
			}
			return
		}
		switch command(message.Text) {
		case "/start":
			h.Start(ctx, b, update)
			return
		case "/cancel":
			h.Cancel(ctx, b, update)
			return
		}
		if !h.service.hasDraft(user.ID) {
			h.Start(ctx, b, update)
			return
		}
		next(ctx, b, update)
	}
}
