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
)

func (h *Handler) getPage(userID int64, result editingResult) navigation.Page {
	if result.state == fsm.Idle {
		return editingResultPage("Нет активного редактирования мероприятия.")
	}
	page := h.prompt(result.state, result.draft)
	page.Keyboard = keyboards.WithScreenToken(page.Keyboard, result.draft.Token)
	page.Key = "admin:event_edit:" + string(result.state)
	page.Admin = true
	page.HistoryMode = navigation.HistoryMatchCurrent
	if result.message != "" {
		page.Text = result.message + "\n\n" + page.Text
	}

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
