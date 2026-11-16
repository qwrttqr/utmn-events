package events

import (
	"context"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Register must precede the general admin callback route: first match wins.
func Register(b *bot.Bot, flow *Flow, guard bot.Middleware, beforeBegin func(int64)) {
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:create_event", bot.MatchTypeExact,
		func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if update.CallbackQuery != nil {
				beforeBegin(update.CallbackQuery.From.ID)
			}
			flow.Begin(ctx, b, update)
		}, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event:", bot.MatchTypePrefix, flow.Handle, guard)
	b.RegisterHandlerMatchFunc(flow.matchMessage, flow.Handle, guard)
}
