package profile

import (
	"context"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"
	"strings"
	"time"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	service     *ProfileService
	beforeBegin func(int64)
}

func NewHandler(service *ProfileService, beforeBegin func(int64)) *Handler {
	return &Handler{service: service, beforeBegin: beforeBegin}
}
func Register(b *bot.Bot, service *ProfileService, beforeBegin func(int64)) {
	h := NewHandler(service, beforeBegin)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:profile", bot.MatchTypeExact, h.Begin)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && parts[0] == "user" && parts[1] == "profile" && parts[3] == "first_name"
	}, h.EditFirstName)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && parts[0] == "user" && parts[1] == "profile" && parts[3] == "last_name"
	}, h.EditLastName)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && parts[0] == "user" && parts[1] == "profile" && parts[3] == "surname"
	}, h.EditSurname)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && parts[0] == "user" && parts[1] == "profile" && parts[3] == "clear_surname"
	}, h.ClearSurname)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && parts[0] == "user" && parts[1] == "profile" && parts[3] == "save"
	}, h.Save)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && parts[0] == "user" && parts[1] == "profile" && parts[3] == "cancel"
	}, h.Cancel)
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
	h.beforeBegin(q.From.ID)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.begin(dbCtx, q.From.ID)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile begin failed", "user_id", q.From.ID, "error", err)
		view.text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.text
	}
	h.show(ctx, b, q.From.ID, q.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.editFirstName(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile editFirstName failed", "user_id", q.From.ID, "error", err)
		view.text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.text
	}
	h.show(ctx, b, q.From.ID, q.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.editLastName(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile editLastName failed", "user_id", q.From.ID, "error", err)
		view.text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.text
	}
	h.show(ctx, b, q.From.ID, q.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.editSurname(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile editSurname failed", "user_id", q.From.ID, "error", err)
		view.text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.text
	}
	h.show(ctx, b, q.From.ID, q.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.clearSurname(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile clearSurname failed", "user_id", q.From.ID, "error", err)
		view.text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.text
	}
	h.show(ctx, b, q.From.ID, q.Message.Message.Chat.ID, view)
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
	view, err := h.service.confirm(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile confirm failed", "user_id", q.From.ID, "error", err)
		view.text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.text
	}
	h.show(ctx, b, q.From.ID, q.Message.Message.Chat.ID, view)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.cancelEdit(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile cancelEdit failed", "user_id", q.From.ID, "error", err)
		view.text = "Не удалось выполнить действие. Попробуйте ещё раз.\n\n" + view.text
	}
	h.show(ctx, b, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) FirstName(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.setFirstName(dbCtx, m.From.ID, m.Text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile setFirstName failed", "user_id", m.From.ID, "error", err)
		view.text = "Не удалось сохранить данные. Отправьте их повторно.\n\n" + view.text
	}
	h.show(ctx, b, m.From.ID, m.Chat.ID, view)
}

func (h *Handler) LastName(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.setLastName(dbCtx, m.From.ID, m.Text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile setLastName failed", "user_id", m.From.ID, "error", err)
		view.text = "Не удалось сохранить данные. Отправьте их повторно.\n\n" + view.text
	}
	h.show(ctx, b, m.From.ID, m.Chat.ID, view)
}

func (h *Handler) Surname(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.setSurname(dbCtx, m.From.ID, m.Text)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "profile setSurname failed", "user_id", m.From.ID, "error", err)
		view.text = "Не удалось сохранить данные. Отправьте их повторно.\n\n" + view.text
	}
	h.show(ctx, b, m.From.ID, m.Chat.ID, view)
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
	h.show(ctx, b, q.From.ID, q.Message.Message.Chat.ID, staleScreen())
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
func (h *Handler) show(ctx context.Context, b *bot.Bot, userID, chatID int64, view screen) {
	page := navigation.Page{Key: "user:profile", Text: view.text, Keyboard: profileKeyboard(view), NoNavigation: view.token != "" && view.state != profileView}
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		view, err := h.service.begin(dbCtx, userID)
		if err != nil {
			return navigation.Page{}, err
		}
		return navigation.Page{Key: "user:profile", Text: view.text, Keyboard: profileKeyboard(view), Restore: page.Restore}, nil
	}
	if err := navigation.Show(ctx, b, chatID, page); err != nil {
		slog.ErrorContext(ctx, "show profile failed", "error", err)
	}
}
func answerCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer profile callback failed", "error", err)
	}
}
