package eventediting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/filters"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
	adminmenu "utmn-events-bot/internal/telegram/routes/admin/menu"
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
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, h.service.machine, eventTitle)
	}, h.Title, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, h.service.machine, eventDescription)
	}, h.Description, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, h.service.machine, eventPhoto)
	}, h.Photo, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, h.service.machine, eventLocation)
	}, h.Location, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, h.service.machine, eventStart)
	}, h.Start, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, h.service.machine, eventEnd)
	}, h.End, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, h.service.machine, eventCapacity)
	}, h.Capacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, h.service.machine, eventTags)
	}, h.Tags, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event_edit:clear_description")) &&
			filters.MatchState(update, h.service.machine, eventDescription)
	}, h.SkipDescription, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event_edit:clear_photo")) &&
			filters.MatchState(update, h.service.machine, eventPhoto)
	}, h.SkipPhoto, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event_edit:unlimited")) &&
			filters.MatchState(update, h.service.machine, eventCapacity)
	}, h.UnlimitedCapacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event_edit:clear_tags")) &&
			filters.MatchState(update, h.service.machine, eventTags)
	}, h.SkipTags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event_edit:save")) &&
			filters.MatchState(update, h.service.machine, eventPreview)
	}, h.Save, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event_edit:cancel"))
	}, h.Cancel, guard)
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
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result := h.service.skipDescription(query.From.ID, token)
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
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result := h.service.skipPhoto(query.From.ID, token)
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
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result := h.service.unlimitedCapacity(query.From.ID, token)
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
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result := h.service.skipTags(query.From.ID, token)
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
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result, err := h.service.confirmEvent(dbCtx, query.From.ID, token)
	cancel()
	chatID := query.Message.Message.Chat.ID
	backToEvents := eventButton{"Мероприятия", "admin:events"}
	if errors.Is(err, domain.ErrEventNotFound) {
		h.showPage(ctx, b, update, chatID, editingResultPage("Мероприятие уже удалено.", backToEvents))
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "event save failed", "user_id", query.From.ID, "error", err)
	} else if event := result.draft.event; result.state == fsm.Idle && event.ID > 0 {
		text := fmt.Sprintf("Мероприятие «%s» (№%d) обновлено.", event.Title, event.ID)
		h.showPage(ctx, b, update, chatID, editingResultPage(text, backToEvents))
		return
	}
	h.show(ctx, b, update, query.From.ID, chatID, result)
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
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result, cancelled := h.service.cancelForm(query.From.ID, token)
	if cancelled {
		h.showPage(ctx, b, update, query.Message.Message.Chat.ID, adminmenu.GetMainPage())
		return
	}
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

func answerEventCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer event callback failed", "error", err)
	}
}

func (h *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, userID, chatID int64, result editingResult) {
	h.showPage(ctx, b, update, chatID, h.getPage(userID, result))
}

func (h *Handler) showPage(ctx context.Context, b *bot.Bot, update *models.Update, chatID int64, page navigation.Page) {
	if err := h.navigator.Show(ctx, b, update, chatID, page); err != nil {
		slog.ErrorContext(ctx, "show event screen failed", "error", err)
	}
}
