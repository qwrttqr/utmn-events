package eventcreation

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

func (f *Handler) getPage(userID int64, result creationResult) navigation.Page {
	if result.state == fsm.Idle {
		return creationResultPage("Нет активного создания мероприятия.")
	}
	page := f.prompt(result.state, result.draft)
	page.Keyboard = keyboards.WithScreenToken(page.Keyboard, result.draft.Token)
	page.Key = "admin:event:" + string(result.state)
	page.Admin = true
	page.HistoryMode = navigation.HistoryMatchCurrent
	if result.message != "" {
		page.Text = result.message + "\n\n" + page.Text
	}

	state, saved := result.state, cloneDraft(result.draft)
	page.Restore = func(context.Context) (navigation.Page, error) {
		restored := f.service.replaceDraft(userID, state, saved)
		return f.getPage(userID, creationResult{state: state, draft: restored}), nil
	}
	return page
}

func creationResultPage(text string) navigation.Page {
	return navigation.Page{Key: "admin:event:result", Text: text, Keyboard: eventKeyboard(), Admin: true, ClearHistory: true}
}

func (f *Handler) prompt(state fsm.StateID, d eventDraft) navigation.Page {
	var view navigation.Page
	switch state {
	case eventTitle:
		view = navigation.Page{Text: "Создание мероприятия\nВведите название."}
	case eventDescription:
		view = f.descriptionPrompt()
	case eventPhoto:
		view = f.photoPrompt()
	case eventLocation:
		view = navigation.Page{Text: "Введите место проведения (адрес, аудитория)."}
	case eventStart:
		view = f.startPrompt()
	case eventEnd:
		view = f.endPrompt(d)
	case eventCapacity:
		view = f.capacityPrompt()
	case eventTags:
		view = f.tagsPrompt()
	case eventPreview:
		view = f.previewPrompt(d)
	}
	cancelText := "Отменить создание"

	addEventButtons(&view, eventAction(cancelText, "cancel"))
	return view
}

func (f *Handler) descriptionPrompt() navigation.Page {
	return navigation.Page{Text: "Введите описание мероприятия.", Keyboard: eventKeyboard(eventAction("Пропустить", "skip_description"))}
}

func (f *Handler) photoPrompt() navigation.Page {
	return navigation.Page{Text: "Отправьте фото мероприятия.", Keyboard: eventKeyboard(eventAction("Пропустить", "skip_photo"))}
}

func (f *Handler) startPrompt() navigation.Page {
	example := f.service.now().In(f.service.location).Truncate(time.Minute).Add(time.Minute)
	return navigation.Page{Text: "Введите дату и время начала: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)}
}

func (f *Handler) endPrompt(d eventDraft) navigation.Page {
	example := d.event.StartsAt.In(f.service.location).Add(time.Hour)
	return navigation.Page{Text: "Введите дату и время окончания: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)}
}

func (f *Handler) capacityPrompt() navigation.Page {
	return navigation.Page{Text: "Введите максимальное количество участников.", Keyboard: eventKeyboard(eventAction("Без ограничений", "unlimited_capacity"))}
}

func (f *Handler) tagsPrompt() navigation.Page {
	return navigation.Page{Text: "Введите теги через точку с запятой: лекция; спорт.", Keyboard: eventKeyboard(eventAction("Пропустить", "skip_tags"))}
}

func (f *Handler) previewPrompt(d eventDraft) navigation.Page {
	capacity := "без ограничений"
	if d.event.Capacity != nil {
		capacity = strconv.Itoa(int(*d.event.Capacity))
	}
	return navigation.Page{
		Text:        fmt.Sprintf("Проверьте мероприятие:\n\n%s\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nУчастников: %s\nТеги: %s", d.event.Title, d.event.Description, d.event.Location, d.event.StartsAt.In(f.service.location).Format(eventDateLayout), d.event.EndsAt.In(f.service.location).Format(eventDateLayout), capacity, strings.Join(d.event.Tags, "; ")),
		PhotoFileID: d.event.PhotoFileID,
		Keyboard:    eventKeyboard(eventAction("Сохранить", "save")),
	}
}
