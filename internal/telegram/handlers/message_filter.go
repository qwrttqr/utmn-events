package handlers

import (
	"strings"

	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
)

func InState(machine *fsm.FSM, state fsm.StateID) func(*models.Update) bool {
	return MatchMessageFrom(func(userID int64) bool {
		return machine.CurrentState(userID) == state
	})
}

func MatchMessageFrom(match func(int64) bool) func(*models.Update) bool {
	return func(update *models.Update) bool {
		message := update.Message
		return message != nil && message.From != nil && match(message.From.ID)
	}
}

func Command(text string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	command, _, _ := strings.Cut(words[0], "@")
	return command
}
