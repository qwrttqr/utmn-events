package profile

import "github.com/go-telegram/bot/models"

func profileKeyboard(view screen) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	if view.state == profileView {
		markup.InlineKeyboard = [][]models.InlineKeyboardButton{
			{{Text: "Изменить имя", CallbackData: "user:profile:" + view.token + ":first_name"}},
			{{Text: "Изменить фамилию", CallbackData: "user:profile:" + view.token + ":last_name"}},
			{{Text: "Изменить отчество", CallbackData: "user:profile:" + view.token + ":surname"}},
		}
		return markup
	}
	if view.state == profileSurname {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Убрать отчество", CallbackData: "user:profile:" + view.token + ":clear_surname"}})
	}
	if view.state == profilePreview {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Сохранить", CallbackData: "user:profile:" + view.token + ":save"}})
	}
	if view.token != "" {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Отменить", CallbackData: "user:profile:" + view.token + ":cancel"}})
	} else {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "👤 Мой профиль", CallbackData: "user:profile"}})
	}
	return markup
}
