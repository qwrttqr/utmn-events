package keyboards

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

const screenTokenSeparator = ":st:"

type ButtonData struct {
	Text         string
	CallbackData string
}

type TokenizedDraft struct {
	Token string
}

func (d *TokenizedDraft) SetScreenToken(token string) {
	d.Token = token
}

func (d TokenizedDraft) IsTokenValid(clickedToken string) bool {
	return clickedToken != "" && d.Token == clickedToken
}

func WithScreenToken(markup *models.InlineKeyboardMarkup, token string) *models.InlineKeyboardMarkup {
	for _, row := range markup.InlineKeyboard {
		for i := range row {
			if row[i].CallbackData != "" {
				row[i].CallbackData += screenTokenSeparator + token
			}
		}
	}
	return markup
}

func ParseScreenCallback(data string) (action, token string, ok bool) {
	action, token, found := strings.Cut(data, screenTokenSeparator)
	return action, token, found && action != "" && token != ""
}

func MatchScreenAction(want string) func(*models.Update) bool {
	return func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		action, _, ok := ParseScreenCallback(q.Data)
		return ok && action == want
	}
}
