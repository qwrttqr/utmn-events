package menu

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"utmn-events-bot/internal/telegram/navigation"
)

func Show(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	err := navigation.Show(ctx, b, update.Message.Chat.ID, Page())
	if err != nil {
		slog.ErrorContext(ctx, "send admin menu failed", "error", err)
	}
}

func Page() navigation.Page {
	return navigation.Page{Key: "admin:menu", Text: "Панель администратора\nВыберите действие:", Keyboard: menuKeyboard(), Admin: true, Root: true}
}
