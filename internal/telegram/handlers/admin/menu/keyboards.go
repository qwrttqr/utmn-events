package menu

import "github.com/go-telegram/bot/models"

func menuKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Создать мероприятие", CallbackData: "admin:create_event"}},
			{{Text: "Мероприятия", CallbackData: "admin:events"}},
			{{Text: "Создать объявление", CallbackData: "admin:announcement"}},
			{{Text: "Теги", CallbackData: "admin:tags"}},
			{{Text: "Пользователи", CallbackData: "admin:users"}},
			{{Text: "Меню пользователя", CallbackData: "user:menu"}},
		},
	}
}
