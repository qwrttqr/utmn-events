package events

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
	"utmn-events-bot/internal/telegram/middleware"
	"utmn-events-bot/internal/telegram/navigation"
	usermenu "utmn-events-bot/internal/telegram/routes/user/menu"
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
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:events", bot.MatchTypeExact, h.run(service.browse))
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:registrations", bot.MatchTypeExact, h.run(service.myRegistrations))
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:search", bot.MatchTypeExact, h.run(service.chooseTags))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "page") &&
			filters.MatchState(update, service.machine, all, mine, results, chooseTags)
	}, h.withClick(h.changePage))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "open") &&
			filters.MatchState(update, service.machine, all, mine, results)
	}, h.withClick(h.openEvent))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "toggle") &&
			filters.MatchState(update, service.machine, chooseTags)
	}, h.withClick(h.toggleTag))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "clear") &&
			filters.MatchState(update, service.machine, chooseTags)
	}, h.withSession(service.clearTags))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "search") &&
			filters.MatchState(update, service.machine, chooseTags)
	}, h.withSession(service.search))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "enroll") &&
			filters.MatchState(update, service.machine, card)
	}, h.withSession(service.enroll))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "cancel") &&
			filters.MatchState(update, service.machine, card)
	}, h.withSession(service.cancelEnrollment))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "keep") &&
			filters.MatchState(update, service.machine, cancelEnrollment)
	}, h.withSession(service.keepEnrollment))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return matchEventAction(update, "confirm") &&
			filters.MatchState(update, service.machine, cancelEnrollment)
	}, h.withSession(service.confirmCancellation))
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackPrefix, bot.MatchTypePrefix, h.run(func(context.Context, int64) (eventsResult, error) {
		return eventsResult{}, errExpiredClick
	}))
}

var errExpiredClick = errors.New("event screen expired")

type eventClick struct {
	eventID int64
	action  string
	token   string
}

func parseEventClick(data string) (eventClick, bool) {
	callback, token, ok := keyboards.ParseScreenCallback(data)
	if !ok {
		return eventClick{}, false
	}
	payload, ok := strings.CutPrefix(callback, callbackPrefix)
	id, action, hasAction := strings.Cut(payload, ":")
	eventID, err := strconv.ParseInt(id, 10, 64)
	if !ok || !hasAction || action == "" || strings.Contains(action, ":") || err != nil || eventID < 0 {
		return eventClick{}, false
	}
	return eventClick{eventID: eventID, action: action, token: token}, true
}

func matchEventAction(update *models.Update, want string) bool {
	if !filters.MatchCallbackPrefix(update, callbackPrefix) {
		return false
	}
	payload := strings.TrimPrefix(update.CallbackQuery.Data, callbackPrefix)
	_, rest, ok := strings.Cut(payload, ":")
	action, _, hasTail := strings.Cut(rest, ":")
	if !ok || !hasTail {
		return false
	}
	if want == "page" || want == "toggle" {
		return strings.HasPrefix(action, want)
	}
	return action == want
}

func (h *Handler) run(action func(context.Context, int64) (eventsResult, error)) bot.HandlerFunc {
	return h.callback(func(ctx context.Context, userID int64, _ string) (eventsResult, error) {
		return action(ctx, userID)
	})
}

func (h *Handler) withSession(action func(context.Context, int64, session) (eventsResult, error)) bot.HandlerFunc {
	return h.withClick(func(ctx context.Context, userID int64, _ fsm.StateID, s session, _ eventClick) (eventsResult, error) {
		return action(ctx, userID, s)
	})
}

func (h *Handler) withClick(action func(context.Context, int64, fsm.StateID, session, eventClick) (eventsResult, error)) bot.HandlerFunc {
	return h.callback(func(ctx context.Context, userID int64, data string) (eventsResult, error) {
		click, ok := parseEventClick(data)
		state, s, exists := fsm.GetDraft[session](h.service.machine, userID)
		if !ok || !exists || !s.IsTokenValid(click.token) || !click.matchesEvent(state, s) {
			return eventsResult{}, errExpiredClick
		}
		return action(ctx, userID, state, s, click)
	})
}

func (click eventClick) matchesEvent(state fsm.StateID, s session) bool {
	if state == card || state == cancelEnrollment {
		return click.eventID > 0 && click.eventID == s.eventID
	}
	if click.action == "open" {
		return click.eventID > 0
	}
	return click.eventID == 0
}

func (h *Handler) changePage(ctx context.Context, userID int64, state fsm.StateID, s session, click eventClick) (eventsResult, error) {
	page, err := strconv.Atoi(strings.TrimPrefix(click.action, "page"))
	if err != nil {
		return eventsResult{}, errExpiredClick
	}
	return h.service.changePage(ctx, userID, state, s, page)
}

func (h *Handler) openEvent(ctx context.Context, userID int64, _ fsm.StateID, s session, click eventClick) (eventsResult, error) {
	return h.service.open(ctx, userID, s, click.eventID)
}

func (h *Handler) toggleTag(ctx context.Context, userID int64, _ fsm.StateID, s session, click eventClick) (eventsResult, error) {
	tagID, err := strconv.ParseInt(strings.TrimPrefix(click.action, "toggle"), 10, 64)
	if err != nil || tagID <= 0 {
		return eventsResult{}, errExpiredClick
	}
	return h.service.toggleTag(ctx, userID, s, tagID)
}

func (h *Handler) callback(action func(context.Context, int64, string) (eventsResult, error)) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
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
		result, err := action(dbCtx, q.From.ID, q.Data)
		cancel()

		var page navigation.Page
		switch {
		case errors.Is(err, errExpiredClick):
			page = expiredPage()
		case errors.Is(err, domain.ErrEventNotFound):
			h.service.Cancel(q.From.ID)
			page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
		case err != nil:
			slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
			page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
		case result.state == fsm.Idle:
			page = usermenu.GetMainPage(h.isAdmin(ctx, q.From.ID))
		default:
			page = h.getPage(q.From.ID, result)
		}
		if err := h.navigator.Show(ctx, b, update, q.Message.Message.Chat.ID, page); err != nil {
			slog.ErrorContext(ctx, "show page failed", "error", err)
		}
	}
}
