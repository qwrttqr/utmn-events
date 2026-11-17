package event_management

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

type Editor interface {
	Edit(context.Context, *bot.Bot, int64, int64, domain.Event, domain.EventField) error
}

type Handler struct {
	service     *EventManagementService
	editor      Editor
	beforeBegin func(int64)
}

func NewHandler(service *EventManagementService, editor Editor, beforeBegin func(int64)) *Handler {
	return &Handler{service: service, editor: editor, beforeBegin: beforeBegin}
}
func Register(b *bot.Bot, service *EventManagementService, editor Editor, guard bot.Middleware, beforeBegin func(int64)) {
	h := NewHandler(service, editor, beforeBegin)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:events", bot.MatchTypeExact, h.List, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:manage:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "page"))
	}, h.Page, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:manage:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "open"))
	}, h.Open, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:manage:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "edit")
	}, h.Edit, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:manage:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "participants")
	}, h.Participants, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:manage:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "field"))
	}, h.Field, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:manage:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "delete")
	}, h.Delete, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:manage:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "keep")
	}, h.Keep, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:manage:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "confirm")
	}, h.ConfirmDelete, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:manage:", bot.MatchTypePrefix, h.StaleCallback, guard)
}
func (h *Handler) List(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	page, edit, err := h.service.list(dbCtx, q.From.ID)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
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
	page, edit, err := h.service.page(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
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
	page, edit, err := h.service.open(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Edit(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	page, edit, err := h.service.edit(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Participants(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	page, edit, err := h.service.participants(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Field(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	page, edit, err := h.service.field(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Delete(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	page, edit, err := h.service.delete(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) Keep(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	page, edit, err := h.service.keep(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}

func (h *Handler) ConfirmDelete(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	page, edit, err := h.service.confirmDelete(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
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
	page, edit, err := h.service.staleCallback(dbCtx, q.From.ID, q.Data)
	cancel()
	if errors.Is(err, domain.ErrEventNotFound) {
		h.service.Cancel(q.From.ID)
		page = navigation.Page{Key: "admin:events:missing", Text: "Мероприятие уже удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	} else if err != nil {
		slog.ErrorContext(ctx, "event_management action failed", "user_id", q.From.ID, "error", err)
		page = navigation.Page{Key: "admin:events:error", Text: "Не удалось загрузить или изменить мероприятие. Попробуйте ещё раз.", Admin: true, Keyboard: retryKeyboard(q.Data)}
	}
	if edit != nil {
		h.beforeBegin(q.From.ID)
		if err := h.editor.Edit(ctx, b, q.From.ID, q.Message.Message.Chat.ID, edit.event, edit.field); err != nil {
			slog.ErrorContext(ctx, "open event editor failed", "error", err)
		}
		return
	}
	if err := navigation.Show(ctx, b, q.Message.Message.Chat.ID, page); err != nil {
		slog.ErrorContext(ctx, "show page failed", "error", err)
	}
}
