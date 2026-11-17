package event_editing

import (
	"github.com/go-telegram/bot/models"
)

func eventKeyboard(view eventScreen) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	for _, button := range view.buttons {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{
			{Text: button.text, CallbackData: button.data},
		})
	}
	return keyboard
}
