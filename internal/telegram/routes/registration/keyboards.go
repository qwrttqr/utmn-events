package registration

import (
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

func registrationPage(text string, buttons ...models.InlineKeyboardButton) navigation.Page {
	page := navigation.Page{Key: "registration", Text: text, Root: true, NoNavigation: true}
	if len(buttons) != 0 {
		page.Keyboard = &models.InlineKeyboardMarkup{}
		for _, button := range buttons {
			page.Keyboard.InlineKeyboard = append(page.Keyboard.InlineKeyboard, []models.InlineKeyboardButton{button})
		}
	}
	return page
}

func registrationButton(text, callback string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: callback}
}

func cancelButton() models.InlineKeyboardButton {
	return registrationButton("Отменить регистрацию", "registration:cancel")
}
