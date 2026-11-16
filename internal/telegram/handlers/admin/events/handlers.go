package events

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (f *Flow) Begin(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	message := query.Message.Message
	if message == nil {
		return
	}
	f.show(ctx, b, query.From.ID, message.Chat.ID, f.begin(query.From.ID))
}

func (f *Flow) Handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	var userID, chatID int64
	var text, photo, callback string
	if query := update.CallbackQuery; query != nil {
		answerEventCallback(ctx, b, query.ID)
		if query.Message.Message == nil {
			return
		}
		userID, chatID, callback = query.From.ID, query.Message.Message.Chat.ID, query.Data
	} else if message := update.Message; message != nil && message.From != nil {
		userID, chatID, text = message.From.ID, message.Chat.ID, message.Text
		if len(message.Photo) > 0 {
			photo = message.Photo[len(message.Photo)-1].FileID
		}
	} else {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.process(dbCtx, userID, text, photo, callback)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event creation failed", "user_id", userID, "error", err)
	}
	f.show(ctx, b, userID, chatID, view)
}

func (f *Flow) matchMessage(update *models.Update) bool {
	m := update.Message
	if m == nil || m.From == nil {
		return false
	}
	// Menu commands are handled by their own routes and discard the draft.
	command := strings.Fields(m.Text)
	if len(command) > 0 && (command[0] == "/start" || command[0] == "/admin") {
		return false
	}
	return f.HasDraft(m.From.ID)
}
