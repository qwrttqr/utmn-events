package event_creation

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"utmn-events-bot/internal/fsm"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	service *EventCreationService
}

func NewHandler(service *EventCreationService) *Handler {
	return &Handler{service: service}
}

func Register(b *bot.Bot, service *EventCreationService, guard bot.Middleware) {
	handler := NewHandler(service)

	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:create_event", bot.MatchTypeExact, handler.Begin, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchMessageState(eventTitle, update) }, handler.Title, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchMessageState(eventDescription, update) }, handler.Description, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchMessageState(eventPhoto, update) }, handler.Photo, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchMessageState(eventLocation, update) }, handler.Location, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchMessageState(eventStart, update) }, handler.Start, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchMessageState(eventEnd, update) }, handler.End, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchMessageState(eventCapacity, update) }, handler.Capacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchMessageState(eventTags, update) }, handler.Tags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return handler.matchCallbackState(eventDescription, "skip_description", update)
	}, handler.SkipDescription, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchCallbackState(eventPhoto, "skip_photo", update) }, handler.SkipPhoto, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return handler.matchCallbackState(eventCapacity, "unlimited_capacity", update)
	}, handler.UnlimitedCapacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchCallbackState(eventTags, "skip_tags", update) }, handler.SkipTags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return handler.matchCallbackState(eventPreview, "save", update) }, handler.Save, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event:cancel", bot.MatchTypeExact, handler.Cancel, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event:", bot.MatchTypePrefix, handler.StaleCallback, guard)
}

func (f *Handler) Begin(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	message := query.Message.Message
	if message == nil {
		return
	}
	f.show(ctx, b, query.From.ID, message.Chat.ID, f.service.begin(query.From.ID))
}

func (f *Handler) Title(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setTitle(message.From.ID, text)
	f.show(ctx, b, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Description(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setDescription(message.From.ID, text)
	f.show(ctx, b, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Photo(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	photo := ""
	if len(message.Photo) > 0 {
		photo = message.Photo[len(message.Photo)-1].FileID
	}
	result := f.service.setPhoto(message.From.ID, photo)
	f.show(ctx, b, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Location(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setLocation(message.From.ID, text)
	f.show(ctx, b, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Start(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setStart(message.From.ID, text)
	f.show(ctx, b, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) End(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setEnd(message.From.ID, text)
	f.show(ctx, b, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Capacity(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setCapacity(message.From.ID, text)
	f.show(ctx, b, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Tags(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setTags(message.From.ID, text)
	f.show(ctx, b, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) SkipDescription(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := f.service.skipDescription(query.From.ID)
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) SkipPhoto(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := f.service.skipPhoto(query.From.ID)
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) UnlimitedCapacity(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := f.service.unlimitedCapacity(query.From.ID)
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) SkipTags(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := f.service.skipTags(query.From.ID)
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) Save(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := f.service.confirmEvent(dbCtx, query.From.ID)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event save failed", "user_id", query.From.ID, "error", err)
	}
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) Cancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := f.service.cancelForm(query.From.ID)
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) StaleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := f.service.staleCallback(query.From.ID)
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) matchMessageState(step fsm.StateID, update *models.Update) bool {
	m := update.Message
	if m == nil || m.From == nil {
		return false
	}
	command := strings.Fields(m.Text)
	if len(command) > 0 && (command[0] == "/start" || command[0] == "/admin") {
		return false
	}
	return f.service.inState(m.From.ID, step)
}

func (f *Handler) matchCallbackState(step fsm.StateID, action string, update *models.Update) bool {
	q := update.CallbackQuery
	return q != nil && q.Data == "admin:event:"+action && f.service.inState(q.From.ID, step)
}

func answerEventCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer event callback failed", "error", err)
	}
}

func (f *Handler) show(ctx context.Context, b *bot.Bot, userID, chatID int64, result creationResult) {
	if err := navigation.Show(ctx, b, chatID, f.page(userID, result)); err != nil {
		slog.ErrorContext(ctx, "show event screen failed", "error", err)
	}
}
