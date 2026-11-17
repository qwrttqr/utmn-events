package events

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	service     *EventsService
	beforeBegin func(int64)
}

func NewHandler(service *EventsService, beforeBegin func(int64)) *Handler {
	return &Handler{service: service, beforeBegin: beforeBegin}
}
func Register(b *bot.Bot, service *EventsService, beforeBegin func(int64)) {
	h := NewHandler(service, beforeBegin)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:events", bot.MatchTypeExact, h.Browse)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:registrations", bot.MatchTypeExact, h.Registrations)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:search", bot.MatchTypeExact, h.ChooseTags)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "page"))
	}, h.Page)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "open"))
	}, h.Open)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "toggle"))
	}, h.ToggleTag)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "clear")
	}, h.ClearTags)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "search")
	}, h.Search)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "enroll")
	}, h.Enroll)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "cancel")
	}, h.CancelEnrollment)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "keep")
	}, h.KeepEnrollment)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "user:event:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "confirm")
	}, h.ConfirmCancellation)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "user:event:", bot.MatchTypePrefix, h.StaleCallback)
}
func (h *Handler) Browse(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	h.beforeBegin(q.From.ID)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.browse(dbCtx, q.From.ID)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Registrations(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	h.beforeBegin(q.From.ID)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.myRegistrations(dbCtx, q.From.ID)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) ChooseTags(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	h.beforeBegin(q.From.ID)
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.chooseTags(dbCtx, q.From.ID)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Page(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.page(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Open(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.open(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) ToggleTag(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.toggleTag(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) ClearTags(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.clearTags(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Search(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.search(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Enroll(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.enroll(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) CancelEnrollment(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.cancelEnrollment(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) KeepEnrollment(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.keepEnrollment(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) ConfirmCancellation(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.confirmCancellation(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) StaleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery
	if q == nil {
		return
	}
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.ErrorContext(ctx, "answer callback failed", "error", err)
	}
	if q.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	page, err := h.service.staleCallback(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "user:events:missing", Text: "Мероприятие уже удалено.", ClearHistory: true, Keyboard: entryKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "events action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "user:events:error", Text: "Не удалось выполнить действие. Попробуйте ещё раз.", Keyboard: retryKeyboard(q.Data)}
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}
