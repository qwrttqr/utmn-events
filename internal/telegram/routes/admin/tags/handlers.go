package tags

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/filters"
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
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:tags", bot.MatchTypeExact, h.callback(func(ctx context.Context, userID int64, _ string) (tagResult, error) {
		return service.begin(ctx, userID)
	}), guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix(tagCallbackPrefix+"create")) &&
			filters.MatchState(update, service.machine, browse)
	}, h.tokenCallback(service.create), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix(tagCallbackPrefix+"assign")) &&
			filters.MatchState(update, service.machine, browse)
	}, h.tokenCallback(service.assign), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, tagCallbackPrefix+"page") &&
			filters.MatchState(update, service.machine, browse, chooseEvent, editTags)
	}, h.numberCallback("page", service.changePage), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, tagCallbackPrefix+"event") &&
			filters.MatchState(update, service.machine, chooseEvent)
	}, h.numberCallback("event", service.selectEvent), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, tagCallbackPrefix+"toggle") &&
			filters.MatchState(update, service.machine, editTags)
	}, h.numberCallback("toggle", service.toggleTag), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix(tagCallbackPrefix+"clear")) &&
			filters.MatchState(update, service.machine, editTags)
	}, h.tokenCallback(service.clearTags), guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix(tagCallbackPrefix+"save")) &&
			filters.MatchState(update, service.machine, editTags)
	}, h.tokenCallback(service.saveTags), guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update != nil && update.Message != nil && filters.MatchState(update, service.machine, createName)
	}, h.Name, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, tagCallbackPrefix, bot.MatchTypePrefix, h.callback(func(_ context.Context, userID int64, _ string) (tagResult, error) {
		if _, _, exists := fsm.GetDraft[session](service.machine, userID); !exists {
			return tagResult{}, errTagSessionMissing
		}
		return tagResult{}, errTagActionStale
	}), guard)
}

func tagNumber(callback, prefix string) (string, int64, bool) {
	action, token, ok := keyboards.ParseScreenCallback(callback)
	value, hasPrefix := strings.CutPrefix(action, tagCallbackPrefix+prefix)
	number, err := strconv.ParseInt(value, 10, 64)
	return token, number, ok && hasPrefix && err == nil && number >= 0
}

func (h *Handler) tokenCallback(run func(context.Context, int64, string) (tagResult, error)) bot.HandlerFunc {
	return h.callback(func(ctx context.Context, userID int64, callback string) (tagResult, error) {
		_, token, ok := keyboards.ParseScreenCallback(callback)
		if !ok {
			return tagResult{}, errTagActionStale
		}
		return run(ctx, userID, token)
	})
}

func (h *Handler) numberCallback(prefix string, run func(context.Context, int64, string, int64) (tagResult, error)) bot.HandlerFunc {
	return h.callback(func(ctx context.Context, userID int64, callback string) (tagResult, error) {
		token, number, ok := tagNumber(callback, prefix)
		if !ok {
			return tagResult{}, errTagActionStale
		}
		return run(ctx, userID, token, number)
	})
}

func (h *Handler) callback(run func(context.Context, int64, string) (tagResult, error)) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
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
		result, err := run(dbCtx, q.From.ID, q.Data)
		cancel()
		h.show(ctx, b, update, q.From.ID, q.Message.Message.Chat.ID, result, err, q.Data)
	}
}

func (h *Handler) Name(ctx context.Context, b *bot.Bot, update *models.Update) {
	m := update.Message
	if m == nil || m.From == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	result, err := h.service.name(dbCtx, m.From.ID, m.Text)
	cancel()
	h.show(ctx, b, update, m.From.ID, m.Chat.ID, result, err, "")
}

func (h *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, userID, chatID int64, result tagResult, err error, callback string) {
	page := navigation.Page{}
	switch {
	case errors.Is(err, errTagSessionMissing):
		page = navigation.Page{Text: "Откройте раздел тегов заново.", Keyboard: tagKeyboard(button{"Теги", "admin:tags"})}
	case errors.Is(err, errTagActionStale):
		page = navigation.Page{Text: "Эта кнопка устарела.", Keyboard: tagKeyboard(button{"Теги", "admin:tags"})}
	case errors.Is(err, errTagNameInvalid):
		page = navigation.Page{Text: "Введите один тег: от 1 до 40 символов, без команд, точки с запятой и переносов строк.", Keyboard: tagKeyboard(button{"Отменить", "admin:menu"})}
	case err != nil:
		slog.ErrorContext(ctx, "tag action failed", "user_id", userID, "error", err)
		switch callback {
		case "admin:tags":
			page = navigation.Page{Text: "Не удалось загрузить теги.", Keyboard: tagKeyboard(button{"Повторить", callback}, button{"В меню", "admin:menu"})}
		case "":
			page = navigation.Page{Text: "Не удалось обратиться к базе данных. Попробуйте ещё раз.\nОтправьте название тега повторно.", Keyboard: tagKeyboard(button{"Отменить", "admin:menu"})}
		default:
			page = navigation.Page{Text: "Не удалось обратиться к базе данных. Попробуйте ещё раз.", Keyboard: tagKeyboard(button{"Повторить", callback}, button{"Отменить", "admin:menu"})}
		}
	case result.state == fsm.Idle:
		page = navigation.Page{Text: "Теги мероприятия сохранены.", ClearHistory: true, Keyboard: tagKeyboard(button{"К тегам", "admin:tags"}, button{"В меню", "admin:menu"})}
	case result.name != "":
		keyboard := tagKeyboard(tagButton("Создать ещё", "create"), tagButton("Назначить мероприятию", "assign"))
		keyboards.WithScreenToken(keyboard, result.session.Token)
		page = navigation.Page{Text: fmt.Sprintf("Тег «%s» сохранён.", result.name), ClearHistory: true, Keyboard: keyboard}
		addTagButtons(&page, button{"К тегам", "admin:tags"})
	default:
		page = tagContent(result)
	}

	state, snapshot := h.service.snapshot(userID)
	if err := h.navigator.Show(ctx, b, update, chatID, h.getPage(userID, page, state, snapshot)); err != nil {
		slog.ErrorContext(ctx, "show tags failed", "error", err)
	}
}
