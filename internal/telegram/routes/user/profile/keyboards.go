package profile

import (
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
)

func profileKeyboard(state fsm.StateID) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	if state == profileView {
		markup.InlineKeyboard = [][]models.InlineKeyboardButton{
			{{Text: "Изменить имя", CallbackData: "user:profile:first_name"}},
			{{Text: "Изменить фамилию", CallbackData: "user:profile:last_name"}},
			{{Text: "Изменить отчество", CallbackData: "user:profile:surname"}},
		}
		return markup
	}
	if state == profileSurname {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Убрать отчество", CallbackData: "user:profile:clear_surname"}})
	}
	if state == profilePreview {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Сохранить", CallbackData: "user:profile:save"}})
	}
	if state != fsm.Idle {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Отменить", CallbackData: "user:profile:cancel"}})
	} else {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "👤 Мой профиль", CallbackData: "user:profile"}})
	}
	return markup
}
