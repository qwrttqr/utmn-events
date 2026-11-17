package menu

import "github.com/go-telegram/bot/models"

func menuKeyboard(admin bool) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Мероприятия", CallbackData: "user:events"}},
		{{Text: "Мои записи", CallbackData: "user:registrations"}},
		{{Text: "Поиск по тегам", CallbackData: "user:search"}},
	}}
	if admin {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Панель администратора", CallbackData: "admin:menu"}})
	}
	return markup
}
