package user

import (
	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/middleware"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/routes/user/events"
	"utmn-events-bot/internal/telegram/routes/user/menu"
	"utmn-events-bot/internal/telegram/routes/user/profile"
)

func Register(b *bot.Bot, navigator *navigation.Navigator, isAdmin middleware.AdminChecker, eventsService *events.EventsService, profileService *profile.ProfileService, finish func(int64)) {
	profile.Register(b, navigator, profileService, isAdmin)
	menu.Register(b, navigator, finish, isAdmin)
	events.Register(b, navigator, eventsService, isAdmin)
}
