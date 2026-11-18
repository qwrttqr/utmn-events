package registration

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/navigation"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type Handler struct {
	navigator *navigation.Navigator
	service   *Service
}

func NewHandler(navigator *navigation.Navigator, service *Service) *Handler {
	return &Handler{navigator: navigator, service: service}
}
func (h *Handler) Register(b *bot.Bot) {
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
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool { return h.matchMessage(registrationConfirmation, update) }, h.Preview)
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
	h.showPrompt(ctx, b, update, m.Chat.ID, h.service.setFirstName(m.From.ID, m.Text))
}

func (h *Handler) LastName(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	h.showPrompt(ctx, b, update, m.Chat.ID, h.service.setLastName(m.From.ID, m.Text))
}

func (h *Handler) Surname(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	h.showPrompt(ctx, b, update, m.Chat.ID, h.service.setSurname(m.From.ID, m.Text))
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
	h.service.begin(user.ID, user.Username)
	h.send(ctx, b, update, message.Chat.ID, firstNamePage())
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
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "registration confirm failed", "user_id", user.ID, "error", err)
		h.send(ctx, b, update, message.Chat.ID, saveErrorPage(result.draft))
		return
	}
	if result.status == registrationCompleted {
		h.showHome(ctx, b, update, message.Chat.ID)
		return
	}
	h.showPrompt(ctx, b, update, message.Chat.ID, result)
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
	h.showPrompt(ctx, b, update, message.Chat.ID, h.service.skipSurname(user.ID))
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
	h.service.Cancel(user.ID)
	h.send(ctx, b, update, message.Chat.ID, cancelledRegistrationPage())
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
	result := h.service.preview(user.ID, user.Username)
	h.send(ctx, b, update, message.Chat.ID, promptPage(result.state, result.draft))
}

func (h *Handler) showPrompt(ctx context.Context, b *bot.Bot, update *models.Update, chatID int64, result registrationResult) {
	if result.status == registrationMissing {
		h.send(ctx, b, update, chatID, missingRegistrationPage())
		return
	}
	if result.status == registrationInvalidName {
		h.send(ctx, b, update, chatID, invalidNamePage(result.state, result.draft))
		return
	}
	h.send(ctx, b, update, chatID, promptPage(result.state, result.draft))
}

func (h *Handler) showHome(ctx context.Context, b *bot.Bot, update *models.Update, chatID int64) {
	if err := h.navigator.ShowHome(ctx, b, update, chatID); err != nil {
		slog.ErrorContext(ctx, "open home after registration failed", "error", err)
	}
}

func (h *Handler) send(ctx context.Context, b *bot.Bot, update *models.Update, chatID int64, page navigation.Page) {
	if err := h.navigator.Show(ctx, b, update, chatID, page); err != nil {
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
