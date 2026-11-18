package users

import (
	"fmt"

	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/pagination"
)

const (
	paginationPrefix = "admin:userlist:"
	promotePrefix    = paginationPrefix + "promote:"
	confirmPrefix    = paginationPrefix + "confirm:"
	cancelPrefix     = paginationPrefix + "cancel:"
)

func retryKeyboard(callback string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Повторить", CallbackData: callback}},
	}}
}

func usersKeyboard(result listResult, adminIDs []int64) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	for i, user := range result.users {
		if !isAdmin(user, adminIDs) {
			markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{
				Text:         fmt.Sprintf("Назначить администратором №%d", result.pageNumber*pagination.Size+i+1),
				CallbackData: fmt.Sprintf("%s%d", promotePrefix, user.TelegramID),
			}})
		}
	}
	pagination.Add(markup, result.pageNumber, result.more, func(action string) string { return paginationPrefix + action })
	return markup
}

func promotionKeyboard(token string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Выдать права администратора", CallbackData: confirmPrefix + token}},
		{{Text: "Отмена", CallbackData: cancelPrefix + token}},
	}}
}
