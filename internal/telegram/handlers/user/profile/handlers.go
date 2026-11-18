package profile

import (
	"context"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
	"strings"
	"time"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/middleware"
	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	navigator *navigation.Navigator
	isAdmin   middleware.AdminChecker
	service   *ProfileService
}

func NewHandler(navigator *navigation.Navigator, service *ProfileService, isAdmin middleware.AdminChecker) *Handler {
	return &Handler{navigator: navigator, service: service, isAdmin: isAdmin}
}
func Register(b *bot.Bot, navigator *navigation.Navigator, service *ProfileService, isAdmin middleware.AdminChecker) {
	h := NewHandler(navigator, service, isAdmin)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile", bot.MatchTypeExact, h.Begin)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile:first_name", bot.MatchTypeExact, h.EditFirstName)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile:last_name", bot.MatchTypeExact, h.EditLastName)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile:surname", bot.MatchTypeExact, h.EditSurname)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile:clear_surname", bot.MatchTypeExact, h.ClearSurname)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile:save", bot.MatchTypeExact, h.Save)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile:cancel", bot.MatchTypeExact, h.Cancel)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchMessage(profileFirstName, update) }, h.FirstName)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchMessage(profileLastName, update) }, h.LastName)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchMessage(profileSurname, update) }, h.Surname)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile:", bot.MatchTypePrefix, h.StaleCallback)
}

func (h *Handler) Begin(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.begin(dbCtx, q.From.ID)
	view := h.page(ctx, q.From.ID, result)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile begin failed", "user_id", q.From.ID, "error", err)
		view.Text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.Text
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) EditFirstName(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	view := h.page(ctx, q.From.ID, h.service.editFirstName(q.From.ID))
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) EditLastName(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	view := h.page(ctx, q.From.ID, h.service.editLastName(q.From.ID))
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) EditSurname(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	view := h.page(ctx, q.From.ID, h.service.editSurname(q.From.ID))
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) ClearSurname(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	view := h.page(ctx, q.From.ID, h.service.clearSurname(q.From.ID))
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Save(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.confirm(dbCtx, q.From.ID)
	view := h.page(ctx, q.From.ID, result)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile confirm failed", "user_id", q.From.ID, "error", err)
		view.Text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.Text
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Cancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	view := h.page(ctx, q.From.ID, h.service.cancelEdit(q.From.ID))
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) FirstName(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	view := h.page(ctx, m.From.ID, h.service.setFirstName(m.From.ID, m.Text))
	h.show(ctx, b, update, m.From.ID, m.Chat.ID, view)
}

func (h *Handler) LastName(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	view := h.page(ctx, m.From.ID, h.service.setLastName(m.From.ID, m.Text))
	h.show(ctx, b, update, m.From.ID, m.Chat.ID, view)
}

func (h *Handler) Surname(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	view := h.page(ctx, m.From.ID, h.service.setSurname(m.From.ID, m.Text))
	h.show(ctx, b, update, m.From.ID, m.Chat.ID, view)
}

func (h *Handler) StaleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, stalePage())
}
func (h *Handler) matchMessage(state fsm.StateID, update *models.Update) bool {
	m := update.Message
	if m == nil || m.From == nil {
		return false
	}
	fields := strings.Fields(m.Text)
	if len(fields) > 0 && (strings.SplitN(fields[0], "@", 2)[0] == "/start" || strings.SplitN(fields[0], "@", 2)[0] == "/admin") {
		return false
	}
	return h.service.inState(m.From.ID, state)
}
func (h *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, userID, chatID int64, page navigation.Page) {
	if page.Key == "" {
		page = profilePage(page.Text, fsm.Idle)
	}
	if !page.Root {
		page.Restore = func(ctx context.Context) (navigation.Page, error) {
			dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			result, err := h.service.begin(dbCtx, userID)
			view := h.page(ctx, userID, result)
			if err != nil {
				return navigation.Page{}, err
			}
			view.Restore = page.Restore
			return view, nil
		}
	}
	if err := h.navigator.Show(ctx, b, update, chatID, page); err != nil {
		slog.ErrorContext(ctx, "show profile failed", "error", err)
	}
}
func answerCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer profile callback failed", "error", err)
	}
}
