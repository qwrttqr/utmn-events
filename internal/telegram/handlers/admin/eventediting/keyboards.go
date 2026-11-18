package eventediting

import (
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

func eventKeyboard(buttons ...eventButton) *models.InlineKeyboardMarkup {
	page := navigation.Page{}
	addEventButtons(&page, buttons...)
	return page.Keyboard
}

func addEventButtons(page *navigation.Page, buttons ...eventButton) {
	if page.Keyboard == nil {
		page.Keyboard = &models.InlineKeyboardMarkup{}
	}
	for _, button := range buttons {
		page.Keyboard.InlineKeyboard = append(page.Keyboard.InlineKeyboard, []models.InlineKeyboardButton{
			{Text: button.text, CallbackData: button.data},
		})
	}
}

func eventAction(text, action string) eventButton {
	return eventButton{text, "admin:event_edit:" + action}
}

type eventButton struct {
	text string
	data string
}
