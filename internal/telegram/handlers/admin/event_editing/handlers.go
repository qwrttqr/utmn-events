package event_editing

import (
	"context"
	"fmt"
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
	view, err := f.service.beginEdit(ctx, userID, event, field)
	if err != nil {
		return err
	}
	f.show(ctx, b, userID, chatID, view)
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
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchCallbackState(eventDescription, "skip", update) }, flow.SkipDescription, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchCallbackState(eventPhoto, "skip", update) }, flow.SkipPhoto, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchCallbackState(eventCapacity, "skip", update) }, flow.UnlimitedCapacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchCallbackState(eventTags, "skip", update) }, flow.SkipTags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return flow.matchCallbackState(eventPreview, "save", update) }, flow.Save, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && parts[0] == "admin" && parts[1] == "event_edit" && parts[3] == "cancel"
	}, flow.Cancel, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event_edit:", bot.MatchTypePrefix, flow.StaleCallback, guard)
}

func (f *Handler) Title(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.setTitle(dbCtx, message.From.ID, text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event title failed", "user_id", message.From.ID, "error", err)
	}
	f.show(ctx, b, message.From.ID, message.Chat.ID, view)
}

func (f *Handler) Description(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.setDescription(dbCtx, message.From.ID, text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event description failed", "user_id", message.From.ID, "error", err)
	}
	f.show(ctx, b, message.From.ID, message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.setPhoto(dbCtx, message.From.ID, photo)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event photo failed", "user_id", message.From.ID, "error", err)
	}
	f.show(ctx, b, message.From.ID, message.Chat.ID, view)
}

func (f *Handler) Location(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.setLocation(dbCtx, message.From.ID, text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event location failed", "user_id", message.From.ID, "error", err)
	}
	f.show(ctx, b, message.From.ID, message.Chat.ID, view)
}

func (f *Handler) Start(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.setStart(dbCtx, message.From.ID, text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event start failed", "user_id", message.From.ID, "error", err)
	}
	f.show(ctx, b, message.From.ID, message.Chat.ID, view)
}

func (f *Handler) End(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.setEnd(dbCtx, message.From.ID, text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event end failed", "user_id", message.From.ID, "error", err)
	}
	f.show(ctx, b, message.From.ID, message.Chat.ID, view)
}

func (f *Handler) Capacity(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.setCapacity(dbCtx, message.From.ID, text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event capacity failed", "user_id", message.From.ID, "error", err)
	}
	f.show(ctx, b, message.From.ID, message.Chat.ID, view)
}

func (f *Handler) Tags(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.setTags(dbCtx, message.From.ID, text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event tags failed", "user_id", message.From.ID, "error", err)
	}
	f.show(ctx, b, message.From.ID, message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.skipDescription(dbCtx, query.From.ID, query.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event skipdescription failed", "user_id", query.From.ID, "error", err)
	}
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.skipPhoto(dbCtx, query.From.ID, query.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event skipphoto failed", "user_id", query.From.ID, "error", err)
	}
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.unlimitedCapacity(dbCtx, query.From.ID, query.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event unlimitedcapacity failed", "user_id", query.From.ID, "error", err)
	}
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.skipTags(dbCtx, query.From.ID, query.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event skiptags failed", "user_id", query.From.ID, "error", err)
	}
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, view)
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
	view, err := f.service.confirmEvent(dbCtx, query.From.ID, query.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event save failed", "user_id", query.From.ID, "error", err)
	}
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.cancelForm(dbCtx, query.From.ID, query.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event cancel failed", "user_id", query.From.ID, "error", err)
	}
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := f.service.staleCallback(dbCtx, query.From.ID, query.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event stalecallback failed", "user_id", query.From.ID, "error", err)
	}
	f.show(ctx, b, query.From.ID, query.Message.Message.Chat.ID, view)
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
	if q == nil || !strings.HasPrefix(q.Data, "admin:event_edit:") {
		return false
	}
	parts := strings.Split(q.Data, ":")
	return len(parts) == 4 && parts[3] == action && f.service.inState(q.From.ID, step)
}

func answerEventCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer event callback failed", "error", err)
	}
}

func (f *Handler) show(ctx context.Context, b *bot.Bot, userID, chatID int64, view eventScreen) {
	snapshot, err := f.service.snapshot(ctx, userID)
	if err != nil {
		reportEditingStorageError(err)
		return
	}
	if err := navigation.Show(ctx, b, chatID, f.page(userID, view, snapshot)); err != nil {
		slog.ErrorContext(ctx, "show event screen failed", "error", err)
	}
}

func (f *Handler) page(userID int64, view eventScreen, snapshot *conversation) navigation.Page {
	page := navigation.Page{Key: "admin:event_edit:result", Text: view.text, PhotoFileID: view.photoID, Keyboard: eventKeyboard(view), Admin: true, ClearHistory: snapshot == nil}
	if snapshot == nil {
		return page
	}
	saved := cloneDraft(*snapshot)
	page.Key = fmt.Sprintf("admin:event_edit:%s:%s", saved.token, saved.state)
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		view, d, err := f.service.restore(ctx, userID, saved)
		if err != nil {
			return navigation.Page{}, err
		}
		return f.page(userID, view, &d), nil
	}
	return page
}
