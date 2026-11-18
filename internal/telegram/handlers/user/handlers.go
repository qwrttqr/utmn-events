package user

import (
	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/handlers/user/events"
	"utmn-events-bot/internal/telegram/handlers/user/menu"
	"utmn-events-bot/internal/telegram/handlers/user/profile"
)

func Register(b *bot.Bot, adminIDs []int64, eventsService *events.EventsService, profileService *profile.ProfileService, finish func(int64)) {
	profile.Register(b, profileService)
	menu.Register(b, adminIDs, finish)
	events.Register(b, eventsService)
}
