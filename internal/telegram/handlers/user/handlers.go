package user

import (
	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/handlers/user/events"
	"utmn-events-bot/internal/telegram/handlers/user/menu"
)

func Register(b *bot.Bot, adminIDs []int64, eventsService *events.EventsService, cancelAdmin func(int64)) func(int64) {
	cancel := func(userID int64) {
		cancelAdmin(userID)
		eventsService.Cancel(userID)
	}
	menu.Register(b, adminIDs, cancel)
	events.Register(b, eventsService, cancelAdmin)
	return cancel
}
