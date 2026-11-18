package registration

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

type Handler struct{ service *RegistrationService }

func NewHandler(service *RegistrationService) *Handler { return &Handler{service: service} }
func Register(b *bot.Bot, service *RegistrationService) {
	h := NewHandler(service)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "registration:start", bot.MatchTypeExact, h.Start)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "registration:restart", bot.MatchTypeExact, h.Start)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "registration:confirm", bot.MatchTypeExact, h.Confirm)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "registration:skip", bot.MatchTypeExact, h.SkipSurname)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "registration:cancel", bot.MatchTypeExact, h.Cancel)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "registration:", bot.MatchTypePrefix, h.Preview)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		m := update.Message
		return m != nil && m.From != nil && command(m.Text) == "/cancel" && h.service.hasDraft(m.From.ID)
	}, h.Cancel)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		m := update.Message
		return m != nil && m.From != nil && command(m.Text) == "/skip" && h.service.hasDraft(m.From.ID)
	}, h.SkipSurname)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchMessage(firstName, update) }, h.FirstName)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchMessage(lastName, update) }, h.LastName)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchMessage(surname, update) }, h.Surname)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchMessage(confirmation, update) }, h.Preview)
}

func (h *Handler) matchMessage(state fsm.StateID, update *models.Update) bool {
	m := update.Message
	if m == nil || m.From == nil {
		return false
	}
	switch command(m.Text) {
	case "/start", "/cancel", "/skip":
		return false
	}
	return h.service.inState(m.From.ID, state)
}
func command(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return strings.SplitN(fields[0], "@", 2)[0]
}

func (h *Handler) FirstName(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	view := resultPage(h.service.setFirstName(m.From.ID, m.Text))
	send(ctx, b, m.Chat.ID, view)
}

func (h *Handler) LastName(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	view := resultPage(h.service.setLastName(m.From.ID, m.Text))
	send(ctx, b, m.Chat.ID, view)
}

func (h *Handler) Surname(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	view := resultPage(h.service.setSurname(m.From.ID, m.Text))
	send(ctx, b, m.Chat.ID, view)
}

func (h *Handler) Start(ctx context.Context, b *bot.Bot, update *models.Update) {
	var user *models.User
	var message *models.Message
	if q := update.CallbackQuery; q != nil {
		user, message = &q.From, q.Message.Message
		answer(ctx, b, q.ID, "")
	} else if m := update.Message; m != nil {
		user, message = m.From, m
	}
	if user == nil || message == nil {
		return
	}
	view := resultPage(h.service.begin(user.ID, user.Username))
	send(ctx, b, message.Chat.ID, view)
}

func (h *Handler) Confirm(ctx context.Context, b *bot.Bot, update *models.Update) {
	var user *models.User
	var message *models.Message
	if q := update.CallbackQuery; q != nil {
		user, message = &q.From, q.Message.Message
		answer(ctx, b, q.ID, "")
	} else if m := update.Message; m != nil {
		user, message = m.From, m
	}
	if user == nil || message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.confirm(dbCtx, user.ID, user.Username)
	view := resultPage(result)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "registration confirm failed", "user_id", user.ID, "error", err)
		view.Text = "Не удалось обратиться к базе данных. Попробуйте ещё раз.\n\n" + view.Text
		view.Root = false
	}
	send(ctx, b, message.Chat.ID, view)
}

func (h *Handler) SkipSurname(ctx context.Context, b *bot.Bot, update *models.Update) {
	var user *models.User
	var message *models.Message
	if q := update.CallbackQuery; q != nil {
		user, message = &q.From, q.Message.Message
		answer(ctx, b, q.ID, "")
	} else if m := update.Message; m != nil {
		user, message = m.From, m
	}
	if user == nil || message == nil {
		return
	}
	view := resultPage(h.service.skipSurname(user.ID))
	send(ctx, b, message.Chat.ID, view)
}

func (h *Handler) Cancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	var user *models.User
	var message *models.Message
	if q := update.CallbackQuery; q != nil {
		user, message = &q.From, q.Message.Message
		answer(ctx, b, q.ID, "")
	} else if m := update.Message; m != nil {
		user, message = m.From, m
	}
	if user == nil || message == nil {
		return
	}
	view := resultPage(h.service.cancelDraft(user.ID))
	send(ctx, b, message.Chat.ID, view)
}

func (h *Handler) Preview(ctx context.Context, b *bot.Bot, update *models.Update) {
	var user *models.User
	var message *models.Message
	if q := update.CallbackQuery; q != nil {
		user, message = &q.From, q.Message.Message
		answer(ctx, b, q.ID, "")
	} else if m := update.Message; m != nil {
		user, message = m.From, m
	}
	if user == nil || message == nil {
		return
	}
	view := resultPage(h.service.preview(user.ID, user.Username))
	send(ctx, b, message.Chat.ID, view)
}

func send(ctx context.Context, b *bot.Bot, chatID int64, page navigation.Page) {
	if err := navigation.Show(ctx, b, chatID, page); err != nil {
		slog.ErrorContext(ctx, "send registration message failed", "error", err)
	}
}

func answer(ctx context.Context, b *bot.Bot, id, text string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: id,
		Text:            text,
	}); err != nil {
		slog.ErrorContext(ctx, "answer registration callback failed", "error", err)
	}
}
