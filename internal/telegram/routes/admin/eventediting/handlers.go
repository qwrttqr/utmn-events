package eventediting

import (
	"context"
	"log/slog"
	"time"
	"utmn-events-bot/internal/fsm"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/handlers"
	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	navigator *navigation.Navigator
	service   *EventEditingService
}

func NewHandler(navigator *navigation.Navigator, service *EventEditingService) *Handler {
	return &Handler{navigator: navigator, service: service}
}

func (h *Handler) Edit(ctx context.Context, b *bot.Bot, update *models.Update, userID, chatID int64, event domain.Event, field domain.EventField) error {
	result, err := h.service.beginEdit(userID, event, field)
	if err != nil {
		return err
	}
	h.show(ctx, b, update, userID, chatID, result)
	return nil
}

func (h *Handler) Register(b *bot.Bot, guard bot.Middleware) {
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, eventTitle), h.Title, guard)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, eventDescription), h.Description, guard)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, eventPhoto), h.Photo, guard)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, eventLocation), h.Location, guard)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, eventStart), h.Start, guard)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, eventEnd), h.End, guard)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, eventCapacity), h.Capacity, guard)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, eventTags), h.Tags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return h.matchCallbackState(eventDescription, "clear_description", update)
	}, h.SkipDescription, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchCallbackState(eventPhoto, "clear_photo", update) }, h.SkipPhoto, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return h.matchCallbackState(eventCapacity, "unlimited_capacity", update)
	}, h.UnlimitedCapacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchCallbackState(eventTags, "clear_tags", update) }, h.SkipTags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchCallbackState(eventPreview, "save", update) }, h.Save, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event_edit:cancel", bot.MatchTypeExact, h.Cancel, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event_edit:", bot.MatchTypePrefix, h.StaleCallback, guard)
}

func (h *Handler) Title(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := h.service.setTitle(message.From.ID, text)
	h.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (h *Handler) Description(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := h.service.setDescription(message.From.ID, text)
	h.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (h *Handler) Photo(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	photo := ""
	if len(message.Photo) > 0 {
		photo = message.Photo[len(message.Photo)-1].FileID
	}
	result := h.service.setPhoto(message.From.ID, photo)
	h.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (h *Handler) Location(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := h.service.setLocation(message.From.ID, text)
	h.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (h *Handler) Start(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := h.service.setStart(message.From.ID, text)
	h.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (h *Handler) End(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := h.service.setEnd(message.From.ID, text)
	h.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (h *Handler) Capacity(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := h.service.setCapacity(message.From.ID, text)
	h.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (h *Handler) Tags(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := h.service.setTags(message.From.ID, text)
	h.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (h *Handler) SkipDescription(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := h.service.skipDescription(query.From.ID)
	h.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (h *Handler) SkipPhoto(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := h.service.skipPhoto(query.From.ID)
	h.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (h *Handler) UnlimitedCapacity(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := h.service.unlimitedCapacity(query.From.ID)
	h.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (h *Handler) SkipTags(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := h.service.skipTags(query.From.ID)
	h.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (h *Handler) Save(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.confirmEvent(dbCtx, query.From.ID)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event save failed", "user_id", query.From.ID, "error", err)
	}
	h.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (h *Handler) Cancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := h.service.cancelForm(query.From.ID)
	h.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (h *Handler) StaleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := h.service.staleCallback(query.From.ID)
	h.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (h *Handler) matchCallbackState(step fsm.StateID, action string, update *models.Update) bool {
	q := update.CallbackQuery
	return q != nil && q.Data == "admin:event_edit:"+action && h.service.inState(q.From.ID, step)
}

func answerEventCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer event callback failed", "error", err)
	}
}

func (h *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, userID, chatID int64, result editingResult) {
	if err := h.navigator.Show(ctx, b, update, chatID, h.getPage(userID, result)); err != nil {
		slog.ErrorContext(ctx, "show event screen failed", "error", err)
	}
}
