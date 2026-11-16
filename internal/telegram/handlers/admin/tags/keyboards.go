package tags

import "github.com/go-telegram/bot/models"

func keyboard(view screen) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	for _, button := range view.buttons {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: button.text, CallbackData: button.data}})
	}
	return markup
}
