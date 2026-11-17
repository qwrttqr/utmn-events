package pagination

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
)

const Size = 8

func Move(current int, action string) (int, bool) {
	if !strings.HasPrefix(action, "page") {
		return current, false
	}
	page, err := strconv.Atoi(strings.TrimPrefix(action, "page"))
	if err != nil || page < 0 || (page != current-1 && page != current+1) {
		return current, false
	}
	return page, true
}

func Add(keyboard *models.InlineKeyboardMarkup, page int, more bool, callback func(string) string) {
	var row []models.InlineKeyboardButton
	if page > 0 {
		row = append(row, models.InlineKeyboardButton{Text: "⬅️", CallbackData: callback(fmt.Sprintf("page%d", page-1))})
	}
	if more {
		row = append(row, models.InlineKeyboardButton{Text: "➡️", CallbackData: callback(fmt.Sprintf("page%d", page+1))})
	}
	if len(row) > 0 {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, row)
	}
}
