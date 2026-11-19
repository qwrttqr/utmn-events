package eventcreation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/filters"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
	adminmenu "utmn-events-bot/internal/telegram/routes/admin/menu"
)

type Handler struct {
	navigator *navigation.Navigator
	service   *EventCreationService
}

func NewHandler(navigator *navigation.Navigator, service *EventCreationService) *Handler {
	return &Handler{navigator: navigator, service: service}
}

func Register(b *bot.Bot, navigator *navigation.Navigator, service *EventCreationService, guard bot.Middleware) {
	handler := NewHandler(navigator, service)

	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:create_event", bot.MatchTypeExact, handler.Begin, guard)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, eventTitle)
	}, handler.Title, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, eventDescription)
	}, handler.Description, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, eventPhoto)
	}, handler.Photo, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, eventLocation)
	}, handler.Location, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, eventStart)
	}, handler.Start, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, eventEnd)
	}, handler.End, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, eventCapacity)
	}, handler.Capacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchState(update, service.machine, eventTags)
	}, handler.Tags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event:skip_description"))
	}, handler.SkipDescription, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event:skip_photo"))
	}, handler.SkipPhoto, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event:unlimited_capacity"))
	}, handler.UnlimitedCapacity, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event:skip_tags"))
	}, handler.SkipTags, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event:save"))
	}, handler.Save, guard)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return filters.MatchCallbackPrefix(update, keyboards.ScreenCallbackPrefix("admin:event:cancel"))
	}, handler.Cancel, guard)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "admin:event:", bot.MatchTypePrefix, handler.StaleCallback, guard)
}

func (f *Handler) Begin(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	message := query.Message.Message
	if message == nil {
		return
	}
	f.show(ctx, b, update, query.From.ID, message.Chat.ID, f.service.begin(query.From.ID))
}

func (f *Handler) Title(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setTitle(message.From.ID, text)
	f.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Description(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setDescription(message.From.ID, text)
	f.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Photo(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	photo := ""
	if len(message.Photo) > 0 {
		photo = message.Photo[len(message.Photo)-1].FileID
	}
	result := f.service.setPhoto(message.From.ID, photo)
	f.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Location(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setLocation(message.From.ID, text)
	f.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Start(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setStart(message.From.ID, text)
	f.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) End(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setEnd(message.From.ID, text)
	f.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Capacity(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setCapacity(message.From.ID, text)
	f.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) Tags(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.Message
	if message == nil || message.From == nil {
		return
	}
	text := message.Text
	result := f.service.setTags(message.From.ID, text)
	f.show(ctx, b, update, message.From.ID, message.Chat.ID, result)
}

func (f *Handler) SkipDescription(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result := f.service.skipDescription(query.From.ID, token)
	f.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) SkipPhoto(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result := f.service.skipPhoto(query.From.ID, token)
	f.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) UnlimitedCapacity(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result := f.service.unlimitedCapacity(query.From.ID, token)
	f.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) SkipTags(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result := f.service.skipTags(query.From.ID, token)
	f.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) Save(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result, err := f.service.confirmEvent(dbCtx, query.From.ID, token)
	cancel()
	if err != nil {
		slog.ErrorContext(ctx, "event save failed", "user_id", query.From.ID, "error", err)
	}
	if result.eventID > 0 {
		page := creationResultPage(fmt.Sprintf("Мероприятие «%s» (№%d) создано.", result.draft.event.Title, result.eventID))
		f.showPage(ctx, b, update, query.Message.Message.Chat.ID, page)
		return
	}
	f.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) Cancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	_, token, _ := keyboards.ParseScreenCallback(query.Data)
	result, cancelled := f.service.cancelForm(query.From.ID, token)
	if cancelled {
		f.showPage(ctx, b, update, query.Message.Message.Chat.ID, adminmenu.GetMainPage())
		return
	}
	f.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func (f *Handler) StaleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	query := update.CallbackQuery
	if query == nil {
		return
	}
	answerEventCallback(ctx, b, query.ID)
	if query.Message.Message == nil {
		return
	}
	result := f.service.staleCallback(query.From.ID)
	f.show(ctx, b, update, query.From.ID, query.Message.Message.Chat.ID, result)
}

func answerEventCallback(ctx context.Context, b *bot.Bot, id string) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: id}); err != nil {
		slog.ErrorContext(ctx, "answer event callback failed", "error", err)
	}
}

func (f *Handler) show(ctx context.Context, b *bot.Bot, update *models.Update, userID, chatID int64, result creationResult) {
	f.showPage(ctx, b, update, chatID, f.getPage(userID, result))
}

func (f *Handler) showPage(ctx context.Context, b *bot.Bot, update *models.Update, chatID int64, page navigation.Page) {
	if err := f.navigator.Show(ctx, b, update, chatID, page); err != nil {
		slog.ErrorContext(ctx, "show event screen failed", "error", err)
	}
}
