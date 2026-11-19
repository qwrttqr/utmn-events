package keyboards

import (
	"crypto/rand"
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

func (d *TokenizedDraft) RotateScreenToken() {
	d.Token = rand.Text()[:12]
}

func (d TokenizedDraft) IsTokenValid(clickedToken string) bool {
	return clickedToken != "" && d.Token == clickedToken
}

func WithScreenToken(markup *models.InlineKeyboardMarkup, token string) *models.InlineKeyboardMarkup {
	for _, row := range markup.InlineKeyboard {
		for i := range row {
			if row[i].CallbackData != "" {
				row[i].CallbackData = ScreenCallbackPrefix(row[i].CallbackData) + token
			}
		}
	}
	return markup
}

func ParseScreenCallback(data string) (action, token string, ok bool) {
	action, token, found := strings.Cut(data, screenTokenSeparator)
	return action, token, found && action != "" && token != ""
}

func ScreenCallbackPrefix(action string) string {
	return action + screenTokenSeparator
}
