package eventmanagement

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
)

type Editor interface {
	Edit(context.Context, *bot.Bot, *models.Update, int64, int64, domain.Event, domain.EventField) error
}

type Handler struct {
	navigator *navigation.Navigator
	service   *EventManagementService
	editor    Editor
}

func NewHandler(navigator *navigation.Navigator, service *EventManagementService, editor Editor) *Handler {
	return &Handler{navigator: navigator, service: service, editor: editor}
}
func Register(b *bot.Bot, navigator *navigation.Navigator, service *EventManagementService, editor Editor, guard bot.Middleware) {
	h := NewHandler(navigator, service, editor)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		return q != nil && (q.Data == "admin:events" || strings.HasPrefix(q.Data, callbackPrefix))
	}, h.handleCallback, guard)
}

func (h *Handler) handleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, edit, err := h.service.process(dbCtx, q.From.ID, q.Data)
	cancel()
	page := h.getPage(q.From.ID, result)
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		if err := h.editor.Edit(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := h.navigator.Show(ctx, b, update, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}
