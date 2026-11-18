package event_editing

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"utmn-events-bot/internal/fsm"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	service *EventEditingService
}

func NewHandler(service *EventEditingService) *Handler {
	return &Handler{service: service}
}

func (f *Handler) Edit(ctx context.Context, b *bot.Bot, userID, chatID int64, event domain.Event, field domain.EventField) error {
	result, err := f.service.beginEdit(userID, event, field)
	if err != nil {
		return err
	}
	f.show(ctx, b, userID, chatID, result)
	return nil
}

func Register(b *bot.Bot, service *EventEditingService, guard bot.Middleware) {
	flow := NewHandler(service)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchMessageState(eventTitle, update) }, flow.Title, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchMessageState(eventDescription, update) }, flow.Description, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchMessageState(eventPhoto, update) }, flow.Photo, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchMessageState(eventLocation, update) }, flow.Location, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchMessageState(eventStart, update) }, flow.Start, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchMessageState(eventEnd, update) }, flow.End, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchMessageState(eventCapacity, update) }, flow.Capacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchMessageState(eventTags, update) }, flow.Tags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return flow.matchCallbackState(eventDescription, "clear_description", update)
	}, flow.SkipDescription, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchCallbackState(eventPhoto, "clear_photo", update) }, flow.SkipPhoto, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return flow.matchCallbackState(eventCapacity, "unlimited_capacity", update)
	}, flow.UnlimitedCapacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchCallbackState(eventTags, "clear_tags", update) }, flow.SkipTags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchCallbackState(eventPreview, "save", update) }, flow.Save, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event_edit:cancel", bot.MatchTypeExact, flow.Cancel, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event_edit:", bot.MatchTypePrefix, flow.StaleCallback, guard)
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
	return q != nil && q.Data == "admin:event_edit:"+action && f.service.inState(q.From.ID, step)
}

func answerEventCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer event callback failed", "error", err)
	}
}

func (f *Handler) show(ctx context.Context, b *bot.Bot, userID, chatID int64, result editingResult) {
	if err := navigation.Show(ctx, b, chatID, f.page(userID, result)); err != nil {
		slog.ErrorContext(ctx, "show event screen failed", "error", err)
	}
}
