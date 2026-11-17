package users

import (
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/pagination"
)

func retryKeyboard(callback string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Повторить", CallbackData: callback}},
	}}
}

func usersKeyboard(s conversation, more bool) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	pagination.Add(markup, s.page, more, func(action string) string { return "admin:userlist:" + s.token + ":" + action })
	return markup
}
