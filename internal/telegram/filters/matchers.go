package filters

import (
	"slices"

	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
)

func MatchState(update *models.Update, machine *fsm.FSM, states ...fsm.StateID) bool {
	if update == nil {
		return false
	}
	if q := update.CallbackQuery; q != nil {
		return slices.Contains(states, machine.CurrentState(q.From.ID))
	}
	m := update.Message
	return m != nil && m.From != nil && slices.Contains(states, machine.CurrentState(m.From.ID))
}
