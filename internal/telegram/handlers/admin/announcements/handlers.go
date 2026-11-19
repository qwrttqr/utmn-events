package announcements

import (
	"context"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	navigator *navigation.Navigator
	service   *Service
}

func NewHandler(navigator *navigation.Navigator, service *Service) *Handler {
	return &Handler{navigator: navigator, service: service}
}

func Register(b *bot.Bot, navigator *navigation.Navigator, service *Service, guard bot.Middleware) {
	h := NewHandler(navigator, service)

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

	b.RegisterHandlerMatchFunc(keyboards.MatchScreenAction("admin:announcement:edit"), h.Edit, guard)
	b.RegisterHandlerMatchFunc(keyboards.MatchScreenAction("admin:announcement:send"), h.Send, guard)
	b.RegisterHandlerMatchFunc(keyboards.MatchScreenAction("admin:announcement:cancel"), h.Cancel, guard)

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
	view := getPage(h.service.begin(q.From.ID))
	h.show(ctx, b, update, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Text(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	view := getPage(h.service.setText(m.From.ID, m.Text))
	h.show(ctx, b, update, m.Chat.ID, view)
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
	_, token, _ := keyboards.ParseScreenCallback(q.Data)
	view := getPage(h.service.edit(q.From.ID, token))
	h.show(ctx, b, update, q.Message.Message.Chat.ID, view)
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
	_, token, _ := keyboards.ParseScreenCallback(q.Data)
	result, job, err := h.service.prepareSend(q.From.ID, token)
	if err != nil {
		slog.ErrorContext(ctx, "prepare announcement failed", "user_id", q.From.ID, "error", err)
		h.show(ctx, b, update, q.Message.Message.Chat.ID, announcementPage("Не удалось начать рассылку объявления.", announcementKeyboard(fsm.Idle), false))
		return
	}
	h.show(ctx, b, update, q.Message.Message.Chat.ID, getPage(result))
	if job == nil {
		return
	}
	go h.service.send(*job, func(ctx context.Context, userID int64, text string) error {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: userID, Text: "Объявление\n\n" + text})
		return err
	}, func(ctx context.Context, userID int64, report report) error {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{ChatID: userID, Text: reportText(report)})
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
	_, token, _ := keyboards.ParseScreenCallback(q.Data)
	view := getPage(h.service.cancelDraft(q.From.ID, token))
	h.show(ctx, b, update, q.Message.Message.Chat.ID, view)
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
	h.show(ctx, b, update, q.Message.Message.Chat.ID, stalePage())
}

func (h *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, chatID int64, page navigation.Page) {
	if err := h.navigator.Show(ctx, b, update, chatID, page); err != nil {
		slog.ErrorContext(ctx, "show announcement screen failed", "error", err)
	}
}

func answerCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer announcement callback failed", "error", err)
	}
}
