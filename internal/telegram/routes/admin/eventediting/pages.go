package eventediting

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
	adminmenu "utmn-events-bot/internal/telegram/routes/admin/menu"
)

func (h *Handler) getPage(userID int64, result editingResult) navigation.Page {
	var message string
	history := navigation.HistoryMatchCurrent
	switch result.status {
	case stepStarted:
		history = navigation.HistoryPush
	case editingCancelled:
		return adminmenu.GetMainPage()
	case noActiveDraft:
		return editingResultPage("Нет активного редактирования мероприятия.")
	case eventMissing:
		return editingResultPage("Мероприятие уже удалено.", eventButton{"Мероприятия", "admin:events"})
	case eventUpdated:
		event := result.draft.event
		return editingResultPage(fmt.Sprintf("Мероприятие «%s» (№%d) обновлено.", event.Title, event.ID), eventButton{"Мероприятия", "admin:events"})
	case stepAlreadyCompleted:
		message = "Этот шаг уже завершён."
	case textRequired:
		message = "Отправьте текст для текущего шага."
	case invalidTitle:
		message = "Название должно быть в одну строку, до 200 символов."
	case descriptionTooLong:
		message = "Описание должно быть не длиннее 2000 символов."
	case photoRequired:
		message = "Отправьте изображение как фото или нажмите «Пропустить»."
	case locationTooLong:
		message = "Место должно быть не длиннее 300 символов."
	case invalidDateFormat:
		message = "Используйте формат ДД.ММ.ГГГГ ЧЧ:ММ."
	case startMustBeFuture:
		message = "Начало должно быть в будущем."
	case endMustFollowStart:
		message = "Окончание должно быть позже начала."
	case invalidCapacity:
		message = "Введите положительное целое число или нажмите «Без ограничений»."
	case tagsTooLong:
		message = "Список тегов должен быть не длиннее 400 символов."
	case invalidTag:
		message = "Каждый тег должен быть в одну строку, до 40 символов."
	case invalidTagCount:
		message = "Введите от 1 до 10 тегов или нажмите «Пропустить»."
	case saveFailed:
		message = "Не удалось сохранить мероприятие. Нажмите «Сохранить» ещё раз."
	case startExpired:
		history = navigation.HistoryPush
		message = "Время начала уже прошло. Введите новую дату и время."
	case capacityBelowRegistrations:
		message = "Количество мест не может быть меньше числа зарегистрированных участников."
	case staleAction:
		message = "Эта кнопка устарела."
	}

	page := h.prompt(result.state, result.draft)
	page.Keyboard = keyboards.WithScreenToken(page.Keyboard, result.draft.Token)
	page.Key = "admin:event_edit:" + string(result.state)
	page.Admin = true
	if message != "" {
		page.Text = message + "\n\n" + page.Text
	}
	page.HistoryMode = history

	state, saved := result.state, cloneDraft(result.draft)
	page.Restore = func(context.Context) (navigation.Page, error) {
		restored := h.service.restore(userID, state, saved)
		return h.getPage(userID, editingResult{state: state, draft: restored}), nil
	}
	return page
}

func editingResultPage(text string, buttons ...eventButton) navigation.Page {
	return navigation.Page{Key: "admin:event_edit:result", Text: text, Keyboard: eventKeyboard(buttons...), Admin: true, ClearHistory: true}
}

func (h *Handler) prompt(state fsm.StateID, d eventDraft) navigation.Page {
	var view navigation.Page
	switch state {
	case eventTitle:
		view = navigation.Page{Text: "Введите новое название."}
	case eventDescription:
		view = h.descriptionPrompt()
	case eventPhoto:
		view = h.photoPrompt()
	case eventLocation:
		view = navigation.Page{Text: "Введите место проведения (адрес, аудитория)."}
	case eventStart:
		view = h.startPrompt()
	case eventEnd:
		view = h.endPrompt(d)
	case eventCapacity:
		view = h.capacityPrompt()
	case eventTags:
		view = h.tagsPrompt()
	case eventPreview:
		view = h.previewPrompt(d)
	}
	view.Text = fmt.Sprintf("Редактирование мероприятия №%d\n\n%s", d.event.ID, view.Text)
	addEventButtons(&view, eventAction("Отменить изменения", "cancel"))
	return view
}

func (h *Handler) descriptionPrompt() navigation.Page {
	return navigation.Page{Text: "Введите описание мероприятия.", Keyboard: eventKeyboard(eventAction("Очистить", "clear_description"))}
}

func (h *Handler) photoPrompt() navigation.Page {
	return navigation.Page{Text: "Отправьте фото мероприятия.", Keyboard: eventKeyboard(eventAction("Очистить", "clear_photo"))}
}

func (h *Handler) startPrompt() navigation.Page {
	example := h.service.now().In(h.service.location).Truncate(time.Minute).Add(time.Minute)
	return navigation.Page{Text: "Введите дату и время начала: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)}
}

func (h *Handler) endPrompt(d eventDraft) navigation.Page {
	example := d.event.StartsAt.In(h.service.location).Add(time.Hour)
	return navigation.Page{Text: "Введите дату и время окончания: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)}
}

func (h *Handler) capacityPrompt() navigation.Page {
	return navigation.Page{Text: "Введите максимальное количество участников.", Keyboard: eventKeyboard(eventAction("Без ограничений", "unlimited"))}
}

func (h *Handler) tagsPrompt() navigation.Page {
	return navigation.Page{Text: "Введите теги через точку с запятой: лекция; спорт.", Keyboard: eventKeyboard(eventAction("Очистить", "clear_tags"))}
}

func (h *Handler) previewPrompt(d eventDraft) navigation.Page {
	capacity := "без ограничений"
	if d.event.Capacity != nil {
		capacity = strconv.Itoa(int(*d.event.Capacity))
	}
	return navigation.Page{
		Text:        fmt.Sprintf("Проверьте мероприятие:\n\n%s\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nУчастников: %s\nТеги: %s", d.event.Title, d.event.Description, d.event.Location, d.event.StartsAt.In(h.service.location).Format(eventDateLayout), d.event.EndsAt.In(h.service.location).Format(eventDateLayout), capacity, strings.Join(d.event.Tags, "; ")),
		PhotoFileID: d.event.PhotoFileID,
		Keyboard:    eventKeyboard(eventAction("Сохранить", "save")),
	}
}
