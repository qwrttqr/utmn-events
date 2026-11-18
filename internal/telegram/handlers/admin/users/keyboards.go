package users

import (
	"fmt"

	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/pagination"
)

const (
	paginationPrefix = "admin:userlist:"
	userPrefix       = paginationPrefix + "user:"
	promotePrefix    = paginationPrefix + "promote:"
	confirmPrefix    = paginationPrefix + "confirm:"
	cancelPrefix     = paginationPrefix + "cancel:"
)

func retryKeyboard(callback string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Повторить", CallbackData: callback}},
	}}
}

func usersKeyboard(result listResult) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	for _, user := range result.users {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{
			Text:         userLabel(user),
			CallbackData: fmt.Sprintf("%s%d", userPrefix, user.TelegramID),
		}})
	}
	pagination.Add(markup, result.pageNumber, result.more, func(action string) string { return paginationPrefix + action })
	return markup
}

func userKeyboard(user domain.User, token string, adminIDs []int64) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	if !isAdmin(user, adminIDs) {
		markup.InlineKeyboard = [][]models.InlineKeyboardButton{{
			{Text: "Назначить администратором", CallbackData: promotePrefix + token},
		}}
	}
	return markup
}

func promotionKeyboard(token string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Выдать права администратора", CallbackData: confirmPrefix + token}},
		{{Text: "Отмена", CallbackData: cancelPrefix + token}},
	}}
}
