package users

import (
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/pagination"
)

const paginationPrefix = "admin:userlist:"

func retryKeyboard(callback string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Повторить", CallbackData: callback}},
	}}
}

func usersKeyboard(pageNumber int, more bool) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	pagination.Add(markup, pageNumber, more, func(action string) string { return paginationPrefix + action })
	return markup
}
