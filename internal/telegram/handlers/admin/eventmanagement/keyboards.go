package eventmanagement

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/pagination"
)

const callbackPrefix = "admin:manage:"

func manageCallback(eventID int64, action string) string {
	return callbackPrefix + strconv.FormatInt(eventID, 10) + ":" + action
}

func parseCallback(callback string) (int64, string, bool) {
	data, ok := strings.CutPrefix(callback, callbackPrefix)
	if !ok {
		return 0, "", false
	}
	id, action, ok := strings.Cut(data, ":")
	if !ok || action == "" || strings.Contains(action, ":") {
		return 0, "", false
	}
	eventID, err := strconv.ParseInt(id, 10, 64)
	return eventID, action, err == nil && eventID >= 0
}

func retryKeyboard(callback string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{{Text: "Повторить", CallbackData: callback}},
		{{Text: "Мероприятия", CallbackData: "admin:events"}},
	}}
}
func listKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Мероприятия", CallbackData: "admin:events"}}}}
}

func eventsKeyboard(s session, events []domain.EventSummary, more bool) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	for _, event := range events {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{
			Text: fmt.Sprintf("№%d %s", event.ID, event.Title), CallbackData: manageCallback(event.ID, "open"),
		}})
	}
	pagination.Add(keyboard, s.page, more, func(action string) string { return manageCallback(0, action) })
	return keyboard
}

func cardKeyboard(s session, event domain.Event) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	add := func(text, action string) {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: manageCallback(s.eventID, action)}})
	}
	add("Редактировать", "edit")
	add("Удалить", "delete")
	add(fmt.Sprintf("Участники (%d)", event.RegistrationCount), "participants")
	return keyboard
}

func participantsKeyboard(s session, more bool) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	pagination.Add(keyboard, s.page, more, func(action string) string { return manageCallback(s.eventID, action) })
	return keyboard
}

func fieldsKeyboard(s session) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	add := func(text, action string) {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: manageCallback(s.eventID, action)}})
	}
	for _, field := range []struct {
		name  string
		field domain.EventField
	}{
		{"Название", domain.EventTitle}, {"Описание", domain.EventDescription}, {"Фото", domain.EventPhoto},
		{"Место", domain.EventLocation}, {"Даты и время", domain.EventDates}, {"Количество мест", domain.EventCapacity}, {"Теги", domain.EventTags},
	} {
		add(field.name, "field"+string(field.field))
	}
	return keyboard
}
func deleteKeyboard(s session) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	add := func(text, action string) {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: manageCallback(s.eventID, action)}})
	}
	add("Подтвердить удаление", "confirm")
	add("Оставить мероприятие", "keep")
	return keyboard
}
