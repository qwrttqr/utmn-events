package admin

import (
	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/middleware"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/routes/admin/announcements"
	"utmn-events-bot/internal/telegram/routes/admin/eventcreation"
	"utmn-events-bot/internal/telegram/routes/admin/eventediting"
	"utmn-events-bot/internal/telegram/routes/admin/eventmanagement"
	"utmn-events-bot/internal/telegram/routes/admin/menu"
	"utmn-events-bot/internal/telegram/routes/admin/tags"
	adminusers "utmn-events-bot/internal/telegram/routes/admin/users"
)

func Register(
	b *bot.Bot,
	navigator *navigation.Navigator,
	isAdmin middleware.AdminChecker,
	eventCreationService *eventcreation.EventCreationService,
	tagsService *tags.TagsService,
	managementService *eventmanagement.EventManagementService,
	usersService *adminusers.UsersService,
	announcementsService *announcements.Service,
	eventEditingService *eventediting.EventEditingService,
	finish func(int64),
) {
	guard := middleware.RequireAdmin(isAdmin)

	eventcreation.Register(b, navigator, eventCreationService, guard)
	tags.Register(b, navigator, tagsService, guard)
	editor := eventediting.NewHandler(navigator, eventEditingService)
	editor.Register(b, guard)
	eventmanagement.Register(b, navigator, managementService, editor, guard)
	adminusers.Register(b, navigator, usersService, guard)
	announcements.Register(b, navigator, announcementsService, guard)
	menu.RegisterCallbacks(b, navigator, guard, finish)
}
