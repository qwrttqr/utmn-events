package tags

import (
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

func tagKeyboard(buttons ...button) *models.InlineKeyboardMarkup {
	page := navigation.Page{}
	addTagButtons(&page, buttons...)
	return page.Keyboard
}

func addTagButtons(page *navigation.Page, buttons ...button) {
	if page.Keyboard == nil {
		page.Keyboard = &models.InlineKeyboardMarkup{}
	}
	for _, button := range buttons {
		page.Keyboard.InlineKeyboard = append(page.Keyboard.InlineKeyboard, []models.InlineKeyboardButton{
			{Text: button.text, CallbackData: button.data},
		})
	}
}

type button struct{ text, data string }

func (s session) button(text, action string) button {
	return button{text, "admin:tag:" + s.token + ":" + action}
}
