package user

import (
	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/handlers/user/events"
	"utmn-events-bot/internal/telegram/handlers/user/menu"
	"utmn-events-bot/internal/telegram/handlers/user/profile"
	"utmn-events-bot/internal/telegram/navigation"
)

func Register(b *bot.Bot, navigator *navigation.Navigator, adminIDs []int64, eventsService *events.EventsService, profileService *profile.ProfileService, finish func(int64)) {
	profile.Register(b, navigator, profileService)
	menu.Register(b, navigator, adminIDs, finish)
	events.Register(b, navigator, eventsService)
}
