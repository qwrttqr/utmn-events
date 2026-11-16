package registration

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"utmn-events-bot/internal/telegram/navigation"
)

func send(ctx context.Context, b *bot.Bot, chatID int64, view screen) {
	if err := navigation.Show(ctx, b, chatID, navigation.Page{
		Key: "registration", Text: view.text, Keyboard: keyboard(view), Root: !view.temporary, NoNavigation: true,
	}); err != nil {
		slog.ErrorContext(ctx, "send registration message failed", "error", err)
	}
}

func answer(ctx context.Context, b *bot.Bot, id, text string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: id,
		Text:            text,
	}); err != nil {
		slog.ErrorContext(ctx, "answer registration callback failed", "error", err)
	}
}
