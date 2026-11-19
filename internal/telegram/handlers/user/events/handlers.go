package events

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/middleware"
	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	navigator *navigation.Navigator
	isAdmin   middleware.AdminChecker
	service   *EventsService
}

func NewHandler(navigator *navigation.Navigator, service *EventsService, isAdmin middleware.AdminChecker) *Handler {
	return &Handler{navigator: navigator, service: service, isAdmin: isAdmin}
}

func Register(b *bot.Bot, navigator *navigation.Navigator, service *EventsService, isAdmin middleware.AdminChecker) {
	h := NewHandler(navigator, service, isAdmin)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		switch q.Data {
		case "user:events", "user:registrations", "user:search":
			return true
		default:
			return strings.HasPrefix(q.Data, callbackPrefix)
		}
	}, h.handleCallback)
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
	result, err := h.service.process(dbCtx, q.From.ID, q.Data)
	cancel()
	page := h.getPage(ctx, q.From.ID, result)
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := h.navigator.Show(ctx, b, update, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}
