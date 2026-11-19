package tags

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/handlers"
	"utmn-events-bot/internal/telegram/navigation"
)

type Handler struct {
	navigator *navigation.Navigator
	service   *TagsService
}

func NewHandler(navigator *navigation.Navigator, service *TagsService) *Handler {
	return &Handler{navigator: navigator, service: service}
}
func Register(b *bot.Bot, navigator *navigation.Navigator, service *TagsService, guard bot.Middleware) {
	h := NewHandler(navigator, service)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:tags", bot.MatchTypeExact, h.List, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:tag:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "create")
	}, h.Create, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:tag:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "assign")
	}, h.Assign, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:tag:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "page"))
	}, h.Page, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:tag:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "event"))
	}, h.SelectEvent, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:tag:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (strings.HasPrefix(parts[3], "toggle"))
	}, h.ToggleTag, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:tag:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "clear")
	}, h.ClearTags, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		q := update.CallbackQuery
		if q == nil || !strings.HasPrefix(q.Data, "admin:tag:") {
			return false
		}
		parts := strings.Split(q.Data, ":")
		return len(parts) == 4 && (parts[3] == "save")
	}, h.Save, guard)

	b.RegisterHandlerMatchFunc(handlers.MatchMessageFrom(service.enteringName), h.Name, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:tag:", bot.MatchTypePrefix, h.StaleCallback, guard)
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
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.begin(dbCtx, q.From.ID)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Create(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	view, err := h.service.create(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Assign(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	view, err := h.service.assign(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
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
	view, err := h.service.changePage(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) SelectEvent(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	view, err := h.service.selectEvent(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
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
	view, err := h.service.toggleTag(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
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
	view, err := h.service.clearTags(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Save(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	view, err := h.service.saveTags(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
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
	view, err := h.service.staleCallback(dbCtx, q.From.ID, q.Data)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "tag action failed", "user_id", q.From.ID, "error", err)
	}
	h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, view)
}

func (h *Handler) Name(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	view, err := h.service.name(dbCtx, m.From.ID, m.Text, "")
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "create tag failed", "user_id", m.From.ID, "error", err)
	}
	h.show(ctx, b, update, m.From.ID, m.Chat.ID, view)
}
func (h *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, userID, chatID int64, result tagResult) {
	state, snapshot := h.service.snapshot(userID)
	if err := h.navigator.Show(ctx, b, update, chatID, h.getPage(userID, result, state, snapshot)); err != nil {
		slog.ErrorContext(ctx, "show tags failed", "error", err)
	}
}
