package eventmanagement

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/filters"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/pagination"
)

type Editor interface {
	Edit(context.Context, *bot.Bot, *models.Update, int64, int64, domain.Event, domain.EventField) error
}

type Handler struct {
	navigator *navigation.Navigator
	service   *Service
	editor    Editor
}

func NewHandler(navigator *navigation.Navigator, service *Service, editor Editor) *Handler {
	return &Handler{navigator: navigator, service: service, editor: editor}
}
func Register(b *bot.Bot, navigator *navigation.Navigator, service *Service, editor Editor, guard bot.Middleware) {
	h := NewHandler(navigator, service, editor)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:events", bot.MatchTypeExact, h.begin, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchAction(update, "page") &&
			filters.MatchState(update, service.machine, list, participants)
	}, h.withClick(h.changePage), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchAction(update, "open") &&
			filters.MatchState(update, service.machine, list)
	}, h.withClick(h.openEvent), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchAction(update, "edit") &&
			filters.MatchState(update, service.machine, card)
	}, h.withClick(h.showFields), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchAction(update, "participants") &&
			filters.MatchState(update, service.machine, card)
	}, h.withClick(h.showParticipants), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchAction(update, "field") &&
			filters.MatchState(update, service.machine, fields)
	}, h.withClick(h.editField), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchAction(update, "delete") &&
			filters.MatchState(update, service.machine, card)
	}, h.withClick(h.showDeleteConfirmation), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchAction(update, "keep") &&
			filters.MatchState(update, service.machine, confirmDelete)
	}, h.withClick(h.openEvent), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchAction(update, "confirm") &&
			filters.MatchState(update, service.machine, confirmDelete)
	}, h.withClick(h.deleteEvent), guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackPrefix, bot.MatchTypePrefix, h.staleCallback, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:manage:", bot.MatchTypePrefix, h.staleCallback, guard)
}

type parsedEventInfo struct {
	eventID int64
	action  string
	token   string
	state   fsm.StateID
	session session
}

func extractEventInfo(data string) (parsedEventInfo, bool) {
	callback, token, ok := keyboards.ParseScreenCallback(data)
	if !ok {
		return parsedEventInfo{}, false
	}
	payload, ok := strings.CutPrefix(callback, callbackPrefix)
	id, action, hasAction := strings.Cut(payload, ":")
	eventID, err := strconv.ParseInt(id, 10, 64)
	if !ok || !hasAction || action == "" || strings.Contains(action, ":") || err != nil || eventID < 0 {
		return parsedEventInfo{}, false
	}
	return parsedEventInfo{eventID: eventID, action: action, token: token}, true
}

func matchAction(update *models.Update, want string) bool {
	if !filters.MatchCallbackPrefix(update, callbackPrefix) {
		return false
	}
	payload := strings.TrimPrefix(update.CallbackQuery.Data, callbackPrefix)
	_, rest, ok := strings.Cut(payload, ":")
	action, _, hasTail := strings.Cut(rest, ":")
	if !ok || !hasTail {
		return false
	}
	if want == "page" || want == "field" {
		return strings.HasPrefix(action, want)
	}
	return action == want
}

func (h *Handler) withClick(action func(context.Context, context.Context, *bot.Bot, *models.Update, parsedEventInfo)) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		q := update.CallbackQuery
		if !answerManagementCallback(ctx, b, q) {
			return
		}
		click, ok := extractEventInfo(q.Data)
		state, s, exists := fsm.GetDraft[session](h.service.machine, q.From.ID)
		if !ok || !exists || !s.IsTokenValid(click.token) || !click.matchesEvent(state, s) {
			h.show(ctx, b, update, q.Message.Message.Chat.ID, expiredPage())
			return
		}
		click.state, click.session = state, s
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		action(ctx, dbCtx, b, update, click)
	}
}

func (click parsedEventInfo) matchesEvent(state fsm.StateID, s session) bool {
	if state == list {
		if click.action == "open" {
			return click.eventID > 0
		}
		return click.eventID == 0
	}
	return click.eventID > 0 && click.eventID == s.eventID
}

func answerManagementCallback(ctx context.Context, b *bot.Bot, q *models.CallbackQuery) bool {
	if q == nil {
		return false
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	return q.Message.Message != nil
}

func (h *Handler) begin(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if !answerManagementCallback(ctx, b, q) {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := h.service.begin(dbCtx, q.From.ID)
	h.showResult(ctx, b, update, result, err)
}

func (h *Handler) changePage(ctx, dbCtx context.Context, b *bot.Bot, update *models.Update, click parsedEventInfo) {
	nextPage, _ := pagination.Move(click.session.page, click.action)
	result, err := h.service.changePage(dbCtx, update.CallbackQuery.From.ID, click.state, click.session, nextPage)
	h.showResult(ctx, b, update, result, err)
}

func (h *Handler) openEvent(ctx, dbCtx context.Context, b *bot.Bot, update *models.Update, click parsedEventInfo) {
	result, err := h.service.openEvent(dbCtx, update.CallbackQuery.From.ID, click.session, click.eventID)
	h.showResult(ctx, b, update, result, err)
}

func (h *Handler) showFields(ctx, dbCtx context.Context, b *bot.Bot, update *models.Update, click parsedEventInfo) {
	result, err := h.service.showFields(dbCtx, update.CallbackQuery.From.ID, click.session)
	h.showResult(ctx, b, update, result, err)
}

func (h *Handler) showParticipants(ctx, dbCtx context.Context, b *bot.Bot, update *models.Update, click parsedEventInfo) {
	result, err := h.service.showParticipants(dbCtx, update.CallbackQuery.From.ID, click.session)
	h.showResult(ctx, b, update, result, err)
}

func (h *Handler) editField(ctx, dbCtx context.Context, b *bot.Bot, update *models.Update, click parsedEventInfo) {
	field := domain.EventField(strings.TrimPrefix(click.action, "field"))
	event, err := h.service.eventForEdit(dbCtx, click.eventID, field)
	if err != nil {
		h.showResult(ctx, b, update, managementResult{}, err)
		return
	}
	q := update.CallbackQuery
	if err := h.editor.Edit(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, event, field); err != nil {
		slog.ErrorContext(ctx, "open event editor failed", "error", err)
	}
}

func (h *Handler) showDeleteConfirmation(ctx, dbCtx context.Context, b *bot.Bot, update *models.Update, click parsedEventInfo) {
	result, err := h.service.showDeleteConfirmation(dbCtx, update.CallbackQuery.From.ID, click.session)
	h.showResult(ctx, b, update, result, err)
}

func (h *Handler) deleteEvent(ctx, dbCtx context.Context, b *bot.Bot, update *models.Update, click parsedEventInfo) {
	q := update.CallbackQuery
	if err := h.service.deleteEvent(dbCtx, q.From.ID, click.eventID); err != nil {
		h.showResult(ctx, b, update, managementResult{}, err)
		return
	}
	page := navigation.Page{Key: "admin:events:deleted", Text: "Мероприятие удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	h.show(ctx, b, update, q.Message.Message.Chat.ID, page)
}

func (h *Handler) staleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if answerManagementCallback(ctx, b, q) {
		h.show(ctx, b, update, q.Message.Message.Chat.ID, expiredPage())
	}
}

func (h *Handler) showResult(ctx context.Context, b *bot.Bot, update *models.Update, result managementResult, err error) {
	q := update.CallbackQuery
	var page navigation.Page
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	} else {
		page = h.getPage(q.From.ID, result)
	}
	h.show(ctx, b, update, q.Message.Message.Chat.ID, page)
}

func (h *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, chatID int64, page navigation.Page) {
	if err := h.navigator.Show(ctx, b, update, chatID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}
