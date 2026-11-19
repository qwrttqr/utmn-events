package announcements

import (
	"context"
	"errors"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/filters"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
	adminmenu "utmn-events-bot/internal/telegram/routes/admin/menu"
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
		return update != nil && update.Message != nil && filters.MatchState(update, service.machine, enterTextState)
	}, h.Text, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:announcement:edit")) &&
			filters.MatchState(update, service.machine, previewState)
	}, h.Edit, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:announcement:send")) &&
			filters.MatchState(update, service.machine, previewState)
	}, h.Send, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:announcement:cancel")) &&
			filters.MatchState(update, service.machine, enterTextState, previewState)
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
	result, started := h.service.begin(q.From.ID)
	if !started {
		h.show(ctx, b, update, q.Message.Message.Chat.ID, broadcastInProgressPage())
		return
	}
	h.show(ctx, b, update, q.Message.Message.Chat.ID, getPage(result))
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
	result, ok := h.service.edit(q.From.ID, token)
	if !ok {
		h.show(ctx, b, update, q.Message.Message.Chat.ID, stalePage())
		return
	}
	h.show(ctx, b, update, q.Message.Message.Chat.ID, getPage(result))
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
	job, err := h.service.prepareSend(q.From.ID, token)
	chatID := q.Message.Message.Chat.ID
	switch {
	case errors.Is(err, errStaleAnnouncement):
		h.show(ctx, b, update, chatID, stalePage())
		return
	case errors.Is(err, errBroadcastInProgress):
		h.show(ctx, b, update, chatID, broadcastInProgressPage())
		return
	case err != nil:
		slog.ErrorContext(ctx, "prepare announcement failed", "user_id", q.From.ID, "error", err)
		h.show(ctx, b, update, chatID, announcementPage("Не удалось начать рассылку объявления.", announcementKeyboard(fsm.Idle), false))
		return
	}
	h.show(ctx, b, update, chatID, announcementPage("Рассылка началась. После завершения придёт отчёт.", announcementKeyboard(fsm.Idle), true))
	go h.service.send(job, func(ctx context.Context, userID int64, text string) error {
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
	if !h.service.cancelDraft(q.From.ID, token) {
		h.show(ctx, b, update, q.Message.Message.Chat.ID, stalePage())
		return
	}
	h.show(ctx, b, update, q.Message.Message.Chat.ID, adminmenu.GetMainPage())
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
