package user

import (
	"context"
	"log/slog"
	"slices"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	adminmenu "utmn-events-bot/internal/telegram/handlers/admin/menu"
	"utmn-events-bot/internal/telegram/navigation"
)

func Register(b *bot.Bot, adminIDs []int64, onOpen func(int64)) {
	adminIDs = slices.Clone(adminIDs)
	menu := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		message := update.Message
		var userID int64
		if update.CallbackQuery != nil {
			query := update.CallbackQuery
			message = query.Message.Message
			userID = query.From.ID
			if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: query.ID}); err != nil {
				slog.ErrorContext(ctx, "answer menu callback failed", "error", err)
			}
		} else if message != nil && message.From != nil {
			userID = message.From.ID
		}
		if message == nil || userID == 0 {
			return
		}
		onOpen(userID)
		page := DefaultPage(userID, adminIDs)
		if update.CallbackQuery != nil && update.CallbackQuery.Data == "user:menu" {
			page = MenuPage(userID, adminIDs)
		}
		if err := navigation.Show(ctx, b, message.Chat.ID, page); err != nil {
			slog.ErrorContext(ctx, "send start message failed", "error", err)
		}
	}
	b.RegisterHandler(bot.HandlerTypeMessageText, "start", bot.MatchTypeCommandStartOnly, menu)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:menu", bot.MatchTypeExact, menu)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "home:menu", bot.MatchTypeExact, menu)
}

func DefaultPage(userID int64, adminIDs []int64) navigation.Page {
	if slices.Contains(adminIDs, userID) {
		return adminmenu.Page()
	}
	return MenuPage(userID, adminIDs)
}

func MenuPage(userID int64, adminIDs []int64) navigation.Page {
	page := navigation.Page{Key: "user:menu", Text: "Меню пользователя\nВыберите действие.", Root: true, Keyboard: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Мероприятия", CallbackData: "user:events"}},
		{{Text: "Мои записи", CallbackData: "user:registrations"}},
		{{Text: "Поиск по тегам", CallbackData: "user:search"}},
	}}}
	if slices.Contains(adminIDs, userID) {
		page.Keyboard.InlineKeyboard = append(page.Keyboard.InlineKeyboard, []models.InlineKeyboardButton{
			{Text: "Панель администратора", CallbackData: "admin:menu"},
		})
	}
	return page
}
