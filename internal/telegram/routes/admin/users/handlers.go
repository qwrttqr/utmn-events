package users

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
)

type Handler struct {
	navigator *navigation.Navigator
	service   *UsersService
}

func NewHandler(navigator *navigation.Navigator, service *UsersService) *Handler {
	return &Handler{navigator: navigator, service: service}
}
func Register(b *bot.Bot, navigator *navigation.Navigator, service *UsersService, guard bot.Middleware) {
	h := NewHandler(navigator, service)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:users", bot.MatchTypeExact, h.List, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, userPrefix) &&
			filters.MatchState(update, service.machine, usersList)
	}, h.User, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix(promoteAction)) &&
			filters.MatchState(update, service.machine, usersDetails)
	}, h.SelectPromotion, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return (filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix(confirmAction)) ||
			filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix(cancelAction))) &&
			filters.MatchState(update, service.machine, usersPromotion)
	}, h.FinishPromotion, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, pagePrefix) &&
			filters.MatchState(update, service.machine, usersList)
	}, h.Page, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, paginationPrefix, bot.MatchTypePrefix, h.StaleCallback, guard)
}

func (h *Handler) User(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil || q.Message.Message == nil {
		return
	}
	action, token, ok := keyboards.ParseScreenCallback(q.Data)
	current, exists := h.listDraft(q.From.ID, token)
	targetID, err := strconv.ParseInt(strings.TrimPrefix(action, userPrefix), 10, 64)
	if !ok || !exists || err != nil || targetID <= 0 {
		answer(ctx, b, q.ID, "Эта кнопка устарела.")
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	selected, err := h.service.selectUser(dbCtx, q.From.ID, targetID, current)
	cancel()
	switch {
	case errors.Is(err, domain.ErrUserNotRegistered):
		answer(ctx, b, q.ID, "Пользователь больше не зарегистрирован.")
	case err != nil:
		slog.ErrorContext(ctx, "load user details failed", "target_id", targetID, "error", err)
		answer(ctx, b, q.ID, "Не удалось загрузить пользователя. Попробуйте ещё раз.")
	case selected.Token == "":
		answer(ctx, b, q.ID, "Эта кнопка устарела.")
	default:
		answer(ctx, b, q.ID, "")
		h.show(ctx, b, update, h.userPage(q.From.ID, selected))
	}
}

func (h *Handler) SelectPromotion(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil || q.Message.Message == nil {
		return
	}
	_, token, parsed := keyboards.ParseScreenCallback(q.Data)
	_, draft, exists := fsm.GetDraft[userSelection](h.service.machine, q.From.ID)
	if !parsed || !exists || !draft.IsTokenValid(token) || isAdmin(draft.target, h.service.adminIDs) {
		answer(ctx, b, q.ID, "Эта кнопка устарела.")
		return
	}
	draft = h.service.beginPromotion(q.From.ID, draft)
	answer(ctx, b, q.ID, "")
	h.show(ctx, b, update, promotionPage(draft, ""))
}

func (h *Handler) FinishPromotion(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil || q.Message.Message == nil {
		return
	}
	action, token, parsed := keyboards.ParseScreenCallback(q.Data)
	_, draft, exists := fsm.GetDraft[userSelection](h.service.machine, q.From.ID)
	if !parsed || !exists || !draft.IsTokenValid(token) {
		answer(ctx, b, q.ID, "Эта кнопка устарела.")
		return
	}
	confirm := action == confirmAction
	answer(ctx, b, q.ID, "")
	if !confirm {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		selected, err := h.service.loadUser(dbCtx, q.From.ID, draft.target.TelegramID, draft.pageNumber)
		cancel()
		if err == nil {
			h.show(ctx, b, update, h.userPage(q.From.ID, selected))
			return
		}
		slog.ErrorContext(ctx, "restore user details failed", "target_id", draft.target.TelegramID, "error", err)
	}

	var notice string
	if confirm {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := h.service.repo.GrantAdmin(dbCtx, draft.target.TelegramID)
		cancel()
		switch {
		case errors.Is(err, domain.ErrUserNotRegistered):
			notice = "Пользователь больше не зарегистрирован.\n\n"
		case err != nil:
			slog.ErrorContext(ctx, "grant admin role failed", "admin_id", q.From.ID, "target_id", draft.target.TelegramID, "error", err)
			h.show(ctx, b, update, promotionPage(draft, "Не удалось выдать права. Попробуйте ещё раз.\n\n"))
			return
		default:
			slog.InfoContext(ctx, "admin role granted", "admin_id", q.From.ID, "target_id", draft.target.TelegramID)
			notice = "Права администратора выданы.\n\n"
		}
	}
	h.service.machine.Clear(q.From.ID)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.loadPage(dbCtx, q.From.ID, draft.pageNumber)
	cancel()
	h.showList(ctx, b, update, result, err, notice, "admin:users")
}

func (h *Handler) List(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answer(ctx, b, q.ID, "")
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.loadPage(dbCtx, q.From.ID, 0)
	cancel()
	h.showList(ctx, b, update, result, err, "", q.Data)
}

func (h *Handler) Page(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if q.Message.Message == nil {
		answer(ctx, b, q.ID, "")
		return
	}
	action, token, parsed := keyboards.ParseScreenCallback(q.Data)
	current, exists := h.listDraft(q.From.ID, token)
	pageNumber, err := strconv.Atoi(strings.TrimPrefix(action, pagePrefix))
	if !parsed || !exists || err != nil {
		h.StaleCallback(ctx, b, update)
		return
	}
	answer(ctx, b, q.ID, "")
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.page(dbCtx, q.From.ID, current, pageNumber)
	cancel()
	h.showList(ctx, b, update, result, err, "", q.Data)
}

func (h *Handler) listDraft(userID int64, token string) (listResult, bool) {
	_, current, exists := fsm.GetDraft[listResult](h.service.machine, userID)
	return current, exists && current.IsTokenValid(token)
}

func (h *Handler) showList(ctx context.Context, b *bot.Bot, update *models.Update, result listResult, err error, notice, retry string) {
	q := update.CallbackQuery
	page := h.getPage(q.From.ID, result)
	if err != nil {
		slog.ErrorContext(ctx, "users action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:users:error", Text: "Не удалось загрузить пользователей.", Admin: true, Keyboard: retryKeyboard(retry)}
	}
	page.Text = notice + page.Text
	h.show(ctx, b, update, page)
}

func (h *Handler) StaleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answer(ctx, b, q.ID, "")
	if q.Message.Message == nil {
		return
	}

	h.show(ctx, b, update, expiredPage())
}

func (h *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, page navigation.Page) {
	if err := h.navigator.Show(ctx, b, update, update.CallbackQuery.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func answer(ctx context.Context, b *bot.Bot, id, message string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id, Text: message}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
}
