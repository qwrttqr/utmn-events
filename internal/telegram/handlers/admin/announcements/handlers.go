package announcements

import (
	"context"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	service     *AnnouncementsService
	beforeBegin func(int64)
}

func NewHandler(service *AnnouncementsService, beforeBegin func(int64)) *Handler {
	return &Handler{service: service, beforeBegin: beforeBegin}
}

func Register(b *bot.Bot, service *AnnouncementsService, guard bot.Middleware, beforeBegin func(int64)) {
	h := NewHandler(service, beforeBegin)

	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:announcement", bot.MatchTypeExact, h.Begin, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		m := update.Message
		if m == nil || m.From == nil {
			return false
		}
		fields := strings.Fields(m.Text)
		if len(fields) > 0 && (fields[0] == "/start" || fields[0] == "/admin") {
			return false
		}
		return service.enteringText(m.From.ID)
	}, h.Text, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		return q != nil && strings.HasPrefix(q.Data, "admin:announcement:") && strings.HasSuffix(q.Data, ":edit")
	}, h.Edit, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		return q != nil && strings.HasPrefix(q.Data, "admin:announcement:") && strings.HasSuffix(q.Data, ":send")
	}, h.Send, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		return q != nil && strings.HasPrefix(q.Data, "admin:announcement:") && strings.HasSuffix(q.Data, ":cancel")
	}, h.Cancel, guard)

	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:announcement:", bot.MatchTypePrefix, h.StaleCallback, guard)
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
	view, err := h.service.begin(ctx, q.From.ID)
	if err != nil {
		slog.ErrorContext(ctx, "begin announcement failed", "user_id", q.From.ID, "error", err)
		h.show(ctx, b, q.Message.Message.Chat.ID, screen{text: "Не удалось начать создание объявления. Попробуйте ещё раз."})
		return
	}
	h.show(ctx, b, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Text(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	view, err := h.service.setText(ctx, m.From.ID, m.Text)
	if err != nil {
		slog.ErrorContext(ctx, "set announcement text failed", "user_id", m.From.ID, "error", err)
		h.show(ctx, b, m.Chat.ID, screen{text: "Не удалось сохранить текст объявления. Отправьте текст повторно."})
		return
	}
	h.show(ctx, b, m.Chat.ID, view)
}

func (h *Handler) Edit(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	view, err := h.service.edit(ctx, q.From.ID, q.Data)
	if err != nil {
		slog.ErrorContext(ctx, "edit announcement failed", "user_id", q.From.ID, "error", err)
		h.show(ctx, b, q.Message.Message.Chat.ID, screen{text: "Не удалось открыть редактирование объявления."})
		return
	}
	h.show(ctx, b, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Send(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	answerCallback(ctx, b, q.ID)
	if q.Message.Message == nil {
		return
	}
	view, job, err := h.service.prepareSend(ctx, q.From.ID, q.Data)
	if err != nil {
		slog.ErrorContext(ctx, "prepare announcement failed", "user_id", q.From.ID, "error", err)
		h.show(ctx, b, q.Message.Message.Chat.ID, screen{text: "Не удалось начать рассылку объявления."})
		return
	}
	h.show(ctx, b, q.Message.Message.Chat.ID, view)
	if job == nil {
		return
	}
	go h.service.send(*job, func(ctx context.Context, userID int64, text string) error {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: userID, Text: text})
		return err
	})
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
	view, err := h.service.cancelDraft(ctx, q.From.ID, q.Data)
	if err != nil {
		slog.ErrorContext(ctx, "cancel announcement failed", "user_id", q.From.ID, "error", err)
		h.show(ctx, b, q.Message.Message.Chat.ID, screen{text: "Не удалось отменить создание объявления."})
		return
	}
	h.show(ctx, b, q.Message.Message.Chat.ID, view)
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
	h.show(ctx, b, q.Message.Message.Chat.ID, staleScreen())
}

func (h *Handler) show(ctx context.Context, b *bot.Bot, chatID int64, view screen) {
	page := navigation.Page{
		Key:          "admin:announcement:" + view.token,
		Text:         view.text,
		Keyboard:     announcementKeyboard(view),
		Admin:        true,
		NoNavigation: true,
		ClearHistory: view.finished,
	}
	if err := navigation.Show(ctx, b, chatID, page); err != nil {
		slog.ErrorContext(ctx, "show announcement screen failed", "error", err)
	}
}

func answerCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer announcement callback failed", "error", err)
	}
}
