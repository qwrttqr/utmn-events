package events

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // The runtime image does not need a system timezone database.
	"unicode/utf8"

	"utmn-events-bot/internal/domain"
)

type Repository interface {
	Create(ctx context.Context, event domain.Event) (int64, error)
}

type UpdateRepository interface {
	Update(context.Context, domain.Event, domain.EventField) error
}

type eventStep int

const (
	eventTitle eventStep = iota
	eventDescription
	eventPhoto
	eventLocation
	eventStart
	eventEnd
	eventCapacity
	eventTags
	eventPreview
)

const eventDateLayout = "02.01.2006 15:04"

type eventDraft struct {
	event           domain.Event
	step            eventStep
	token           string
	returnToPreview bool
	updateField     domain.EventField
}

type eventButton struct {
	text string
	data string
}

type eventScreen struct {
	text    string
	buttons []eventButton
	photoID string
}

type Flow struct {
	mu       sync.Mutex
	drafts   map[int64]eventDraft
	repo     Repository
	location *time.Location
	now      func() time.Time
}

func New(repo Repository) (*Flow, error) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return nil, fmt.Errorf("load event timezone: %w", err)
	}
	return &Flow{drafts: make(map[int64]eventDraft), repo: repo, location: location, now: time.Now}, nil
}

func (f *Flow) Cancel(userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.drafts, userID)
}

func (f *Flow) HasDraft(userID int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.drafts[userID]
	return ok
}

func (f *Flow) begin(userID int64) eventScreen {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := eventDraft{token: rand.Text()}
	f.drafts[userID] = d
	return f.prompt(d)
}

func (f *Flow) process(ctx context.Context, userID int64, text, photoID, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.drafts[userID]
	if !ok {
		return eventScreen{text: "Нет активного создания мероприятия.", buttons: []eventButton{{"В меню администратора", "admin:menu"}}}, nil
	}
	if callback != "" {
		// Bind actions to this draft and step, so old keyboards cannot publish
		// or skip fields in a later conversation.
		parts := strings.Split(callback, ":")
		if len(parts) != 5 || parts[2] != d.token || parts[3] != strconv.Itoa(int(d.step)) {
			view := f.prompt(d)
			view.text = "Эта кнопка устарела.\n\n" + view.text
			return view, nil
		}
		switch parts[4] {
		case "cancel":
			delete(f.drafts, userID)
			if d.updateField != "" {
				return eventScreen{text: "Изменения отменены.", buttons: []eventButton{{"Мероприятия", "admin:events"}}}, nil
			}
			return eventScreen{text: "Создание мероприятия отменено.", buttons: []eventButton{{"В меню администратора", "admin:menu"}}}, nil
		case "save":
			if d.step != eventPreview {
				return f.prompt(d), nil
			}
			if (d.updateField == "" || d.updateField == domain.EventDates) && !d.event.StartsAt.After(f.now()) {
				d.step = eventStart
				d.returnToPreview = true
				f.drafts[userID] = d
				return f.invalid(d, "Время начала уже прошло. Введите новую дату и время."), nil
			}
			id := d.event.ID
			var err error
			if d.updateField != "" {
				updater, ok := f.repo.(UpdateRepository)
				if !ok {
					return f.invalid(d, "Редактирование недоступно."), errors.New("event repository does not support updates")
				}
				err = updater.Update(ctx, d.event, d.updateField)
			} else {
				id, err = f.repo.Create(ctx, d.event)
			}
			if err != nil {
				if errors.Is(err, domain.ErrCapacityBelowRegistrations) {
					return f.invalid(d, "Количество мест не может быть меньше числа зарегистрированных участников."), nil
				}
				if errors.Is(err, domain.ErrEventNotFound) {
					delete(f.drafts, userID)
					return eventScreen{text: "Мероприятие уже удалено.", buttons: []eventButton{{"Мероприятия", "admin:events"}}}, nil
				}
				return f.invalid(d, "Не удалось сохранить мероприятие. Нажмите «Сохранить» ещё раз."), fmt.Errorf("save event: %w", err)
			}
			delete(f.drafts, userID)
			if d.updateField != "" {
				return eventScreen{text: fmt.Sprintf("Мероприятие «%s» (№%d) обновлено.", d.event.Title, id), buttons: []eventButton{{"Мероприятия", "admin:events"}}}, nil
			}
			return eventScreen{text: fmt.Sprintf("Мероприятие «%s» (№%d) создано.", d.event.Title, id), buttons: []eventButton{{"В меню администратора", "admin:menu"}}}, nil
		case "skip":
			if d.step != eventDescription && d.step != eventPhoto && d.step != eventCapacity && d.step != eventTags {
				return f.prompt(d), nil
			}
			switch d.step {
			case eventDescription:
				d.event.Description = ""
			case eventPhoto:
				d.event.PhotoFileID = ""
			case eventCapacity:
				d.event.Capacity = nil
			case eventTags:
				d.event.Tags = nil
			}
			f.advance(&d)
			f.drafts[userID] = d
			return f.prompt(d), nil
		default:
			return f.prompt(d), nil
		}
	}

	text = strings.TrimSpace(text)
	if d.step == eventPreview {
		return f.prompt(d), nil
	}
	if d.step != eventPhoto && (text == "" || strings.HasPrefix(text, "/")) {
		return f.invalid(d, "Отправьте текст для текущего шага."), nil
	}
	switch d.step {
	case eventTitle:
		if utf8.RuneCountInString(text) > 200 || strings.ContainsAny(text, "\r\n") {
			return f.invalid(d, "Название должно быть в одну строку, до 200 символов."), nil
		}
		d.event.Title = text
	case eventDescription:
		if utf8.RuneCountInString(text) > 2000 {
			return f.invalid(d, "Описание должно быть не длиннее 2000 символов."), nil
		}
		d.event.Description = text
	case eventPhoto:
		if photoID == "" {
			return f.invalid(d, "Отправьте изображение как фото или нажмите «Пропустить»."), nil
		}
		d.event.PhotoFileID = photoID
	case eventLocation:
		if utf8.RuneCountInString(text) > 300 {
			return f.invalid(d, "Место должно быть не длиннее 300 символов."), nil
		}
		d.event.Location = text
	case eventStart, eventEnd:
		value, err := time.ParseInLocation(eventDateLayout, text, f.location)
		if err != nil {
			return f.invalid(d, "Используйте формат ДД.ММ.ГГГГ ЧЧ:ММ."), nil
		}
		if d.step == eventStart {
			if !value.After(f.now()) {
				return f.invalid(d, "Начало должно быть в будущем."), nil
			}
			d.event.StartsAt = value.UTC()
		} else {
			if !value.After(d.event.StartsAt) {
				return f.invalid(d, "Окончание должно быть позже начала."), nil
			}
			d.event.EndsAt = value.UTC()
		}
	case eventCapacity:
		value, err := strconv.ParseInt(text, 10, 32)
		if err != nil || value <= 0 {
			return f.invalid(d, "Введите положительное целое число или нажмите «Без ограничений»."), nil
		}
		capacity := int32(value)
		d.event.Capacity = &capacity
	case eventTags:
		if utf8.RuneCountInString(text) > 400 {
			return f.invalid(d, "Список тегов должен быть не длиннее 400 символов."), nil
		}
		seen := make(map[string]bool)
		var tags []string
		for _, tag := range strings.Split(text, ";") {
			tag = strings.ToLower(strings.TrimSpace(tag))
			if tag == "" || seen[tag] {
				continue
			}
			if utf8.RuneCountInString(tag) > 40 || strings.ContainsAny(tag, "\r\n") {
				return f.invalid(d, "Каждый тег должен быть в одну строку, до 40 символов."), nil
			}
			seen[tag] = true
			tags = append(tags, tag)
		}
		if len(tags) == 0 || len(tags) > 10 {
			return f.invalid(d, "Введите от 1 до 10 тегов или нажмите «Пропустить»."), nil
		}
		d.event.Tags = tags
	}
	f.advance(&d)
	f.drafts[userID] = d
	return f.prompt(d), nil
}

func (f *Flow) invalid(d eventDraft, text string) eventScreen {
	view := f.prompt(d)
	view.text = text + "\n\n" + view.text
	return view
}

func (f *Flow) advance(d *eventDraft) {
	previous := d.step
	if d.updateField != "" {
		if previous == eventStart {
			d.step = eventEnd
		} else {
			d.step = eventPreview
		}
		return
	}
	d.step++
	if previous == eventEnd && d.returnToPreview {
		d.step = eventPreview
		d.returnToPreview = false
	}
}

func (f *Flow) prompt(d eventDraft) eventScreen {
	view := eventScreen{}
	add := func(text, action string) {
		if d.updateField != "" && text == "Пропустить" {
			text = "Очистить"
		}
		view.buttons = append(view.buttons, eventButton{text, fmt.Sprintf("admin:event:%s:%d:%s", d.token, d.step, action)})
	}
	switch d.step {
	case eventTitle:
		view.text = "Создание мероприятия\nВведите название."
		if d.updateField != "" {
			view.text = "Введите новое название."
		}
	case eventDescription:
		view.text = "Введите описание мероприятия."
		add("Пропустить", "skip")
	case eventPhoto:
		view.text = "Отправьте фото мероприятия."
		add("Пропустить", "skip")
	case eventLocation:
		view.text = "Введите место проведения (адрес, аудитория)."
	case eventStart:
		example := f.now().In(f.location).Truncate(time.Minute).Add(time.Minute)
		view.text = "Введите дату и время начала: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)
	case eventEnd:
		example := d.event.StartsAt.In(f.location).Add(time.Hour)
		view.text = "Введите дату и время окончания: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)
	case eventCapacity:
		view.text = "Введите максимальное количество участников."
		add("Без ограничений", "skip")
	case eventTags:
		view.text = "Введите теги через точку с запятой: лекция; спорт."
		add("Пропустить", "skip")
	case eventPreview:
		capacity := "без ограничений"
		if d.event.Capacity != nil {
			capacity = strconv.Itoa(int(*d.event.Capacity))
		}
		view.text = fmt.Sprintf("Проверьте мероприятие:\n\n%s\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nУчастников: %s\nТеги: %s", d.event.Title, d.event.Description, d.event.Location, d.event.StartsAt.In(f.location).Format(eventDateLayout), d.event.EndsAt.In(f.location).Format(eventDateLayout), capacity, strings.Join(d.event.Tags, "; "))
		view.photoID = d.event.PhotoFileID
		add("Сохранить", "save")
	}
	if d.updateField != "" {
		view.text = fmt.Sprintf("Редактирование мероприятия №%d\n\n%s", d.event.ID, view.text)
		add("Отменить изменения", "cancel")
	} else {
		add("Отменить создание", "cancel")
	}
	return view
}
