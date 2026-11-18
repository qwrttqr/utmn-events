package admin

import (
	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/telegram/handlers/admin/announcements"
	"utmn-events-bot/internal/telegram/handlers/admin/event_creation"
	"utmn-events-bot/internal/telegram/handlers/admin/event_editing"
	"utmn-events-bot/internal/telegram/handlers/admin/event_management"
	"utmn-events-bot/internal/telegram/handlers/admin/menu"
	"utmn-events-bot/internal/telegram/handlers/admin/tags"
	adminusers "utmn-events-bot/internal/telegram/handlers/admin/users"
	"utmn-events-bot/internal/telegram/middleware"
)

func Register(
	b *bot.Bot,
	adminIDs []int64,
	eventCreationService *event_creation.EventCreationService,
	tagsService *tags.TagsService,
	managementService *event_management.EventManagementService,
	usersService *adminusers.UsersService,
	announcementsService *announcements.AnnouncementsService,
	eventEditingService *event_editing.EventEditingService,
	finish func(int64),
) {
	guard := middleware.RequireAdmin(adminIDs)
	menu.RegisterCommand(b, guard, finish)

	event_creation.Register(b, eventCreationService, guard)
	tags.Register(b, tagsService, guard)
	event_editing.Register(b, eventEditingService, guard)
	event_management.Register(b, managementService, event_editing.NewHandler(eventEditingService), guard)
	adminusers.Register(b, usersService, guard)
	announcements.Register(b, announcementsService, guard)
	menu.RegisterCallbacks(b, guard, finish)
}
