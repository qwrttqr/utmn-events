package profile

import (
	"context"
	"log/slog"
	"time"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/filters"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/middleware"
	"utmn-events-bot/internal/telegram/navigation"
	usermenu "utmn-events-bot/internal/telegram/routes/user/menu"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("user:profile:first_name")) &&
			filters.MatchState(update, service.machine, profileView)
	}, h.editField(profileFirstName, firstNameField))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("user:profile:last_name")) &&
			filters.MatchState(update, service.machine, profileView)
	}, h.editField(profileLastName, lastNameField))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("user:profile:surname")) &&
			filters.MatchState(update, service.machine, profileView)
	}, h.editField(profileSurname, surnameField))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("user:profile:clear_surname")) &&
			filters.MatchState(update, service.machine, profileSurname)
	}, h.withDraft(func(_ context.Context, userID int64, d draft) (profileResult, error) {
		return service.clearSurname(userID, d), nil
	}))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("user:profile:save")) &&
			filters.MatchState(update, service.machine, profilePreview)
	}, h.withDraft(service.confirm))

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("user:profile:cancel")) &&
			filters.MatchState(update, service.machine, profileFirstName, profileLastName, profileSurname, profilePreview)
	}, h.withDraft(func(_ context.Context, userID int64, _ draft) (profileResult, error) {
		service.cancelEdit(userID)
		return profileResult{state: fsm.Idle}, nil
	}))

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, profileFirstName)
	}, h.setName(firstNameField))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, profileLastName)
	}, h.setName(lastNameField))
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, profileSurname)
	}, h.setName(surnameField))
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
	cancel()
	page := h.getPage(result)
	if err != nil {
		slog.ErrorContext(ctx, "profile begin failed", "user_id", q.From.ID, "error", err)
		page = profilePage("Не удалось выполнить действие. Попробуйте ещё раз.", fsm.Idle)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, page)
}

func (h *Handler) editField(state fsm.StateID, field profileField) bot.HandlerFunc {
	return h.withDraft(func(_ context.Context, userID int64, d draft) (profileResult, error) {
		return h.service.edit(userID, d, state, field), nil
	})
}

func (h *Handler) withDraft(action func(context.Context, int64, draft) (profileResult, error)) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		q := update.CallbackQuery
		if q == nil {
			return
		}
		answerCallback(ctx, b, q.ID)
		if q.Message.Message == nil {
			return
		}
		chatID := q.Message.Message.Chat.ID
		_, token, ok := keyboards.ParseScreenCallback(q.Data)
		_, d, exists := fsm.GetDraft[draft](h.service.machine, q.From.ID)
		if !ok || !exists || !d.IsTokenValid(token) {
			h.show(ctx, b, update, q.From.ID, chatID, stalePage())
			return
		}

		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		result, err := action(dbCtx, q.From.ID, d)
		cancel()
		var page navigation.Page
		if err != nil {
			slog.ErrorContext(ctx, "profile action failed", "user_id", q.From.ID, "error", err)
			result.message = "Не удалось выполнить действие. Попробуйте ещё раз."
			page = h.getPage(result)
		} else if result.state == fsm.Idle {
			page = usermenu.GetMainPage(h.isAdmin(ctx, q.From.ID))
		} else {
			page = h.getPage(result)
		}
		h.show(ctx, b, update, q.From.ID, chatID, page)
	}
}

func (h *Handler) setName(field profileField) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		m := update.Message
		if m == nil || m.From == nil {
			return
		}
		result := h.service.setName(m.From.ID, m.Text, field)
		h.show(ctx, b, update, m.From.ID, m.Chat.ID, h.getPage(result))
	}
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
func (h *Handler) show(
	ctx context.Context,
	b *bot.Bot,
	update *models.Update,
	userID, chatID int64,
	page navigation.Page,
) {
	if page.Key == "" {
		page = profilePage(page.Text, fsm.Idle)
	}
	if !page.Root {
		page.Restore = func(ctx context.Context) (navigation.Page, error) {
			dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			result, err := h.service.begin(dbCtx, userID)
			view := h.getPage(result)
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
