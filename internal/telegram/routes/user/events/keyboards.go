package events

import (
	"fmt"
	"strconv"
	"time"

	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/pagination"
)

const callbackPrefix = "user:event:"

func eventCallback(eventID int64, action string) string {
	return callbackPrefix + strconv.FormatInt(eventID, 10) + ":" + action
}

func pageCallback(action string) string {
	return eventCallback(0, action)
}

func retryKeyboard(callback string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Повторить", CallbackData: callback}},
		{{Text: "Мероприятия", CallbackData: "user:events"}},
	}}
}
func entryKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Мероприятия", CallbackData: "user:events"}}}}
}

func tagsKeyboard(s session, tags []domain.Tag, more bool) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	callback := func(action string) string { return eventCallback(0, action) }
	add := func(text, action string) {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: callback(action)}})
	}
	for _, tag := range tags {
		mark := "[ ] "
		if _, selected := s.selected[tag.ID]; selected {
			mark = "[x] "
		}
		add(mark+tag.Name, "toggle"+strconv.FormatInt(tag.ID, 10))
	}
	pagination.Add(markup, s.page, more, pageCallback)
	if len(s.selected) > 0 {
		add("Снять выбор", "clear")
	}
	add("Найти мероприятия", "search")
	return markup
}
func eventsKeyboard(s session, events []domain.EventSummary, more bool) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	for _, event := range events {
		title := []rune(event.Title)
		if len(title) > 45 {
			title = append(title[:45], '…')
		}
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: fmt.Sprintf("№%d %s", event.ID, string(title)), CallbackData: eventCallback(event.ID, "open")}})
	}
	pagination.Add(markup, s.page, more, pageCallback)
	return markup
}
func cardKeyboard(state fsm.StateID, s session, event domain.Event, enrolled bool, now time.Time) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	callback := func(action string) string { return eventCallback(s.eventID, action) }
	add := func(text, action string) {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: callback(action)}})
	}
	if state == cancelEnrollment {
		add("Подтвердить отмену записи", "confirm")
		add("Оставить запись", "keep")
	} else if enrolled {
		add("Отменить запись", "cancel")
	} else if event.StartsAt.After(now) && (event.Capacity == nil || event.RegistrationCount < int64(*event.Capacity)) {
		add("Записаться", "enroll")
	}
	return markup
}
