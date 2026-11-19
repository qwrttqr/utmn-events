package registration

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
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
		if update.Message != nil && command(update.Message.Text) == "/start" {
			if !registered {
				h.Start(ctx, b, update)
			} else if h.service.inState(user.ID, fsm.Idle) {
				h.showHome(ctx, b, update, message.Chat.ID)
			}
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
		if !h.service.hasDraft(user.ID) {
			h.Start(ctx, b, update)
			return
		}
		next(ctx, b, update)
	}
}

func command(text string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	name, _, _ := strings.Cut(words[0], "@")
	return name
}
