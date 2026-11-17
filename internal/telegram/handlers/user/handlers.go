package user

import (
	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/handlers/user/events"
	"utmn-events-bot/internal/telegram/handlers/user/menu"
	"utmn-events-bot/internal/telegram/handlers/user/profile"
)

func Register(b *bot.Bot, adminIDs []int64, eventsService *events.EventsService, profileService *profile.ProfileService, cancelAdmin func(int64)) func(int64) {
	cancel := func(userID int64) {
		cancelAdmin(userID)
		eventsService.Cancel(userID)
		profileService.Cancel(userID)
	}
	profile.Register(b, profileService, cancel)
	menu.Register(b, adminIDs, cancel)
	events.Register(b, eventsService, cancelAdmin)
	return cancel
}
