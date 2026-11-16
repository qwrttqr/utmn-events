package menu

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"utmn-events-bot/internal/telegram/navigation"
)

func Callback(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
	}); err != nil {
		slog.ErrorContext(ctx, "answer admin callback failed", "error", err)
	}

	message := query.Message.Message
	if message == nil {
		return
	}
	page := navigation.Page{Key: query.Data, Admin: true}
	switch query.Data {
	case "admin:menu":
		page = Page()
	case "admin:announcement":
		page.Text = "Создание объявления пока недоступно."
	default:
		page.Text = "Неизвестное действие. Вернитесь в меню администратора."
	}
	err := navigation.Show(ctx, b, message.Chat.ID, page)
	if err != nil {
		slog.ErrorContext(ctx, "edit admin menu failed", "error", err)
	}
}
