package announcements

import "github.com/go-telegram/bot/models"

func announcementKeyboard(view screen) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	if view.state == previewState {
		markup.InlineKeyboard = append(markup.InlineKeyboard,
			[]models.InlineKeyboardButton{{Text: "Отправить", CallbackData: "admin:announcement:" + view.token + ":send"}},
			[]models.InlineKeyboardButton{{Text: "Изменить текст", CallbackData: "admin:announcement:" + view.token + ":edit"}},
		)
	}
	if view.token != "" {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Отменить", CallbackData: "admin:announcement:" + view.token + ":cancel"}})
	} else {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Панель администратора", CallbackData: "admin:menu"}})
	}
	return markup
}
