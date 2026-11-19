package filters

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

func MatchCallbackPrefix(update *models.Update, prefix string) bool {
	return update != nil && update.CallbackQuery != nil &&
		strings.HasPrefix(update.CallbackQuery.Data, prefix)
}

func MatchCallbackData(update *models.Update, data string) bool {
	return update != nil && update.CallbackQuery != nil && update.CallbackQuery.Data == data
}
