package event_management

import (
	"fmt"
	"strconv"

	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/pagination"
)

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
	add := func(text, action string) {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: "admin:manage:" + s.token + ":" + action}})
	}
	for _, event := range events {
		add(fmt.Sprintf("№%d %s", event.ID, event.Title), "open"+strconv.FormatInt(event.ID, 10))
	}
	pagination.Add(keyboard, s.page, more, func(action string) string { return "admin:manage:" + s.token + ":" + action })
	return keyboard
}

func cardKeyboard(s session, event domain.Event) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	add := func(text, action string) {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: "admin:manage:" + s.token + ":" + action}})
	}
	add("Редактировать", "edit")
	add("Удалить", "delete")
	add(fmt.Sprintf("Участники (%d)", event.RegistrationCount), "participants")
	return keyboard
}

func participantsKeyboard(s session, more bool) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	pagination.Add(keyboard, s.page, more, func(action string) string { return "admin:manage:" + s.token + ":" + action })
	return keyboard
}

func fieldsKeyboard(s session) *models.InlineKeyboardMarkup {
	keyboard := &models.InlineKeyboardMarkup{}
	add := func(text, action string) {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: "admin:manage:" + s.token + ":" + action}})
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
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: "admin:manage:" + s.token + ":" + action}})
	}
	add("Подтвердить удаление", "confirm")
	add("Оставить мероприятие", "keep")
	return keyboard
}
