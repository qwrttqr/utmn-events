package event_creation

import (
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

func eventAction(text, action string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: "admin:event:" + action}
}

func eventKeyboard(buttons ...models.InlineKeyboardButton) *models.InlineKeyboardMarkup {
	page := navigation.Page{}
	addEventButtons(&page, buttons...)
	return page.Keyboard
}

func addEventButtons(page *navigation.Page, buttons ...models.InlineKeyboardButton) {
	if page.Keyboard == nil {
		page.Keyboard = &models.InlineKeyboardMarkup{}
	}
	for _, button := range buttons {
		page.Keyboard.InlineKeyboard = append(page.Keyboard.InlineKeyboard, []models.InlineKeyboardButton{button})
	}
}
