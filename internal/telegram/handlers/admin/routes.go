package admin

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/handlers/admin/events"
	"utmn-events-bot/internal/telegram/handlers/admin/events/manage"
	"utmn-events-bot/internal/telegram/handlers/admin/menu"
	"utmn-events-bot/internal/telegram/handlers/admin/tags"
	adminusers "utmn-events-bot/internal/telegram/handlers/admin/users"
	"utmn-events-bot/internal/telegram/middleware"
)

// Register returns the cleanup function used when opening the user panel.
func Register(b *bot.Bot, adminIDs []int64, flow *events.Flow, tagFlow *tags.Flow, management *manage.Flow, users *adminusers.Flow) func(int64) {
	guard := middleware.RequireAdmin(adminIDs)
	cancel := func(id int64) { flow.Cancel(id); tagFlow.Cancel(id); management.Cancel(id); users.Cancel(id) }
	b.RegisterHandler(bot.HandlerTypeMessageText, "admin", bot.MatchTypeCommandStartOnly,
		func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.Message != nil && update.Message.From != nil {
				cancel(update.Message.From.ID)
			}
			menu.Show(ctx, b, update)
		}, guard)
	events.Register(b, flow, guard, cancel)
	tags.Register(b, tagFlow, guard, cancel)
	manage.Register(b, management, guard, cancel)
	adminusers.Register(b, users, guard, cancel)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:", bot.MatchTypePrefix,
		func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.CallbackQuery != nil {
				cancel(update.CallbackQuery.From.ID)
			}
			menu.Callback(ctx, b, update)
		}, guard)
	return cancel
}
