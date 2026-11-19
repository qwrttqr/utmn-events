package registration

import (
	"context"
	"log/slog"
	"time"
	"utmn-events-bot/internal/telegram/handlers"
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
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, firstName), h.FirstName)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, lastName), h.LastName)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, surname), h.Surname)
	b.RegisterHandlerMatchFunc(handlers.InState(h.service.machine, registrationConfirmation), h.Preview)
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
