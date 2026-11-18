package user

import (
	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/handlers/user/events"
	"utmn-events-bot/internal/telegram/handlers/user/menu"
	"utmn-events-bot/internal/telegram/handlers/user/profile"
	"utmn-events-bot/internal/telegram/middleware"
	"utmn-events-bot/internal/telegram/navigation"
)

func Register(b *bot.Bot, navigator *navigation.Navigator, isAdmin middleware.AdminChecker, eventsService *events.EventsService, profileService *profile.ProfileService, finish func(int64)) {
	profile.Register(b, navigator, profileService, isAdmin)
	menu.Register(b, navigator, finish, isAdmin)
	events.Register(b, navigator, eventsService, isAdmin)
}
