package eventcreation

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"utmn-events-bot/internal/fsm"
	adminmenu "utmn-events-bot/internal/telegram/handlers/admin/menu"
	"utmn-events-bot/internal/telegram/navigation"
)

func (f *Handler) getPage(userID int64, result creationResult) navigation.Page {
	var message string
	history := navigation.HistoryMatchCurrent
	switch result.status {
	case stepStarted:
		history = navigation.HistoryPush
	case creationCancelled:
		return adminmenu.GetMainPage()
	case noActiveDraft:
		return creationResultPage("Нет активного создания мероприятия.")
	case eventCreated:
		return creationResultPage(fmt.Sprintf("Мероприятие «%s» (№%d) создано.", result.draft.event.Title, result.eventID))
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
	case staleAction:
		message = "Эта кнопка устарела."
	}

	page := f.prompt(result.state, result.draft)
	page.Key = "admin:event:" + string(result.state)
	page.Admin = true
	if message != "" {
		page.Text = message + "\n\n" + page.Text
	}
	page.HistoryMode = history

	state, saved := result.state, cloneDraft(result.draft)
	page.Restore = func(context.Context) (navigation.Page, error) {
		f.service.restore(userID, state, saved)
		return f.getPage(userID, creationResult{state: state, draft: saved}), nil
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
