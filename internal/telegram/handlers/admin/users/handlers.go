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
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, promotePrefix, bot.MatchTypePrefix, h.SelectPromotion, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, confirmPrefix, bot.MatchTypePrefix, h.FinishPromotion, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, cancelPrefix, bot.MatchTypePrefix, h.FinishPromotion, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		action, ok := strings.CutPrefix(q.Data, paginationPrefix)
		return ok && strings.HasPrefix(action, "page") && !strings.Contains(action, ":")
	}, h.Page, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, paginationPrefix, bot.MatchTypePrefix, h.StaleCallback, guard)
}

func (h *Handler) SelectPromotion(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil || q.Message.Message == nil {
		return
	}
	targetID, err := strconv.ParseInt(strings.TrimPrefix(q.Data, promotePrefix), 10, 64)
	if err != nil {
		answer(ctx, b, q.ID, "Эта кнопка устарела.")
		return
	}
	draft, ok := h.service.selectPromotion(q.From.ID, targetID)
	if !ok {
		answer(ctx, b, q.ID, "Эта кнопка устарела.")
		return
	}
	answer(ctx, b, q.ID, "")
	h.show(ctx, b, update, promotionPage(draft, ""))
}

func (h *Handler) FinishPromotion(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil || q.Message.Message == nil {
		return
	}
	token, confirm := strings.CutPrefix(q.Data, confirmPrefix)
	if !confirm {
		token = strings.TrimPrefix(q.Data, cancelPrefix)
	}
	draft, ok := h.service.pendingPromotion(q.From.ID, token)
	if !ok {
		answer(ctx, b, q.ID, "Эта кнопка устарела.")
		return
	}
	answer(ctx, b, q.ID, "")

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
	answer(ctx, b, q.ID, "")
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.page(dbCtx, q.From.ID, strings.TrimPrefix(q.Data, paginationPrefix))
	cancel()
	h.showList(ctx, b, update, result, err, "", q.Data)
}

func (h *Handler) showList(ctx context.Context, b *bot.Bot, update *models.Update, result listResult, err error, notice, retry string) {
	q := update.CallbackQuery
	page := h.page(q.From.ID, result)
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
