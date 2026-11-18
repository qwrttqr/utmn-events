package announcements

import (
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
)

func announcementKeyboard(state fsm.StateID) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	if state == previewState {
		markup.InlineKeyboard = append(markup.InlineKeyboard,
			[]models.InlineKeyboardButton{{Text: "Отправить", CallbackData: "admin:announcement:send"}},
			[]models.InlineKeyboardButton{{Text: "Изменить текст", CallbackData: "admin:announcement:edit"}},
		)
	}
	if state == textState || state == previewState {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Отменить", CallbackData: "admin:announcement:cancel"}})
	} else {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "Панель администратора", CallbackData: "admin:menu"}})
	}
	return markup
}
