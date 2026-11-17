package event_editing

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
)

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
	event       domain.Event
	step        eventStep
	token       string
	updateField domain.EventField
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

type EventEditingService struct {
	mu       sync.Mutex
	machine  *fsm.FSM[string, eventDraft]
	repo     Repository
	location *time.Location
	now      func() time.Time
}

func NewService(repository Repository) (*EventEditingService, error) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return nil, fmt.Errorf("load event timezone: %w", err)
	}
	flow := &EventEditingService{repo: repository, machine: fsm.NewInMemoryFSM[string, eventDraft]("idle"), location: location, now: time.Now}
	return flow, nil
}

func (f *EventEditingService) Cancel(userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clear(userID)
}

func cloneDraft(d eventDraft) eventDraft {
	d.event.Tags = slices.Clone(d.event.Tags)
	if d.event.Capacity != nil {
		capacity := *d.event.Capacity
		d.event.Capacity = &capacity
	}
	return d
}

var editingConversationStates = [...]fsm.StateID{"admin.event_editing.title", "admin.event_editing.description", "admin.event_editing.photo", "admin.event_editing.location", "admin.event_editing.start", "admin.event_editing.end", "admin.event_editing.capacity", "admin.event_editing.tags", "admin.event_editing.preview"}

func reportEditingStorageError(err error) {
	slog.Error("conversation storage failed", "error", err)
}

func (f *EventEditingService) load(ctx context.Context, userID int64) (eventDraft, bool, error) {
	state, err := f.machine.CurrentState(ctx, userID)
	if err != nil {
		return eventDraft{}, false, err
	}
	value, err := f.machine.GetData(ctx, userID, "session")
	if errors.Is(err, fsm.ErrUserDataNotFound) {
		return eventDraft{}, false, nil
	}
	if err != nil {
		return eventDraft{}, false, err
	}
	for index, expected := range editingConversationStates {
		if state == expected {
			value.step = eventStep(index)
			return value, true, nil
		}
	}
	return eventDraft{}, false, fmt.Errorf("unexpected conversation state %q", state)
}

func (f *EventEditingService) save(ctx context.Context, userID int64, value eventDraft) error {
	index := int(value.step)
	if index < 0 || index >= len(editingConversationStates) {
		return errors.New("invalid conversation step")
	}
	if err := f.machine.SetData(ctx, userID, "session", value); err != nil {
		return err
	}
	return f.machine.Transition(ctx, userID, editingConversationStates[index])
}

func (f *EventEditingService) clear(userID int64) {
	if err := f.machine.Finish(context.Background(), userID); err != nil {
		reportEditingStorageError(err)
	}
}

func (f *EventEditingService) completeStep(ctx context.Context, userID int64, d eventDraft) (eventScreen, error) {
	if d.step == eventStart {
		d.step = eventEnd
	} else {
		d.step = eventPreview
	}
	if err := f.save(ctx, userID, d); err != nil {
		return eventScreen{}, err
	}
	return f.prompt(d), nil
}

func (f *EventEditingService) cancelForm(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 5 || parts[0] != "admin" || parts[1] != "event_edit" || parts[2] != d.token || parts[3] != strconv.Itoa(int(d.step)) || parts[4] != "cancel" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}

	f.clear(userID)
	return eventScreen{text: "Изменения отменены.", buttons: []eventButton{{"Мероприятия", "admin:events"}}}, nil
}

func (f *EventEditingService) button(d eventDraft, text, action string) eventButton {
	if text == "Пропустить" {
		text = "Очистить"
	}
	return eventButton{text, fmt.Sprintf("admin:event_edit:%s:%d:%s", d.token, d.step, action)}
}

func (f *EventEditingService) prompt(d eventDraft) eventScreen {
	var view eventScreen
	switch d.step {
	case eventTitle:
		view = f.titlePrompt(d)
	case eventDescription:
		view = f.descriptionPrompt(d)
	case eventPhoto:
		view = f.photoPrompt(d)
	case eventLocation:
		view = f.locationPrompt(d)
	case eventStart:
		view = f.startPrompt(d)
	case eventEnd:
		view = f.endPrompt(d)
	case eventCapacity:
		view = f.capacityPrompt(d)
	case eventTags:
		view = f.tagsPrompt(d)
	case eventPreview:
		view = f.previewPrompt(d)
	}
	view.text = fmt.Sprintf("Редактирование мероприятия №%d\n\n%s", d.event.ID, view.text)
	view.buttons = append(view.buttons, f.button(d, "Отменить изменения", "cancel"))
	return view
}

func (f *EventEditingService) invalid(d eventDraft, message string) eventScreen {
	view := f.prompt(d)
	view.text = message + "\n\n" + view.text
	return view
}

func (f *EventEditingService) setTitle(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	if d.step != eventTitle {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return f.invalid(d, "Отправьте текст для текущего шага."), nil
	}

	if utf8.RuneCountInString(text) > 200 || strings.ContainsAny(text, "\r\n") {
		return f.invalid(d, "Название должно быть в одну строку, до 200 символов."), nil
	}
	d.event.Title = text
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) titlePrompt(d eventDraft) eventScreen {
	return eventScreen{text: "Введите новое название."}
}

func (f *EventEditingService) setDescription(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	if d.step != eventDescription {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return f.invalid(d, "Отправьте текст для текущего шага."), nil
	}

	if utf8.RuneCountInString(text) > 2000 {
		return f.invalid(d, "Описание должно быть не длиннее 2000 символов."), nil
	}
	d.event.Description = text
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) skipDescription(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 5 || parts[0] != "admin" || parts[1] != "event_edit" || parts[2] != d.token || parts[3] != strconv.Itoa(int(d.step)) || parts[4] != "skip" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.step != eventDescription {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	d.event.Description = ""
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) descriptionPrompt(d eventDraft) eventScreen {
	return eventScreen{text: "Введите описание мероприятия.", buttons: []eventButton{f.button(d, "Пропустить", "skip")}}
}

func (f *EventEditingService) setPhoto(ctx context.Context, userID int64, photo string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	if d.step != eventPhoto {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	if photo == "" {
		return f.invalid(d, "Отправьте изображение как фото или нажмите «Пропустить»."), nil
	}
	d.event.PhotoFileID = photo
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) skipPhoto(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 5 || parts[0] != "admin" || parts[1] != "event_edit" || parts[2] != d.token || parts[3] != strconv.Itoa(int(d.step)) || parts[4] != "skip" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.step != eventPhoto {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	d.event.PhotoFileID = ""
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) photoPrompt(d eventDraft) eventScreen {
	return eventScreen{text: "Отправьте фото мероприятия.", buttons: []eventButton{f.button(d, "Пропустить", "skip")}}
}

func (f *EventEditingService) setLocation(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	if d.step != eventLocation {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return f.invalid(d, "Отправьте текст для текущего шага."), nil
	}

	if utf8.RuneCountInString(text) > 300 {
		return f.invalid(d, "Место должно быть не длиннее 300 символов."), nil
	}
	d.event.Location = text
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) locationPrompt(eventDraft) eventScreen {
	return eventScreen{text: "Введите место проведения (адрес, аудитория)."}
}

func (f *EventEditingService) setStart(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	if d.step != eventStart {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return f.invalid(d, "Отправьте текст для текущего шага."), nil
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return f.invalid(d, "Используйте формат ДД.ММ.ГГГГ ЧЧ:ММ."), nil
	}
	if !value.After(f.now()) {
		return f.invalid(d, "Начало должно быть в будущем."), nil
	}
	d.event.StartsAt = value.UTC()
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) setEnd(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	if d.step != eventEnd {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return f.invalid(d, "Отправьте текст для текущего шага."), nil
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return f.invalid(d, "Используйте формат ДД.ММ.ГГГГ ЧЧ:ММ."), nil
	}
	if !value.After(d.event.StartsAt) {
		return f.invalid(d, "Окончание должно быть позже начала."), nil
	}
	d.event.EndsAt = value.UTC()
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) startPrompt(eventDraft) eventScreen {
	example := f.now().In(f.location).Truncate(time.Minute).Add(time.Minute)
	return eventScreen{text: "Введите дату и время начала: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)}
}

func (f *EventEditingService) endPrompt(d eventDraft) eventScreen {
	example := d.event.StartsAt.In(f.location).Add(time.Hour)
	return eventScreen{text: "Введите дату и время окончания: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)}
}

func (f *EventEditingService) setCapacity(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	if d.step != eventCapacity {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return f.invalid(d, "Отправьте текст для текущего шага."), nil
	}

	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil || value <= 0 {
		return f.invalid(d, "Введите положительное целое число или нажмите «Без ограничений»."), nil
	}
	capacity := int32(value)
	d.event.Capacity = &capacity
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) unlimitedCapacity(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 5 || parts[0] != "admin" || parts[1] != "event_edit" || parts[2] != d.token || parts[3] != strconv.Itoa(int(d.step)) || parts[4] != "skip" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.step != eventCapacity {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	d.event.Capacity = nil
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) capacityPrompt(d eventDraft) eventScreen {
	return eventScreen{text: "Введите максимальное количество участников.", buttons: []eventButton{f.button(d, "Без ограничений", "skip")}}
}

func (f *EventEditingService) setTags(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	if d.step != eventTags {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return f.invalid(d, "Отправьте текст для текущего шага."), nil
	}

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
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) skipTags(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 5 || parts[0] != "admin" || parts[1] != "event_edit" || parts[2] != d.token || parts[3] != strconv.Itoa(int(d.step)) || parts[4] != "skip" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.step != eventTags {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	d.event.Tags = nil
	return f.completeStep(ctx, userID, d)
}

func (f *EventEditingService) tagsPrompt(d eventDraft) eventScreen {
	return eventScreen{text: "Введите теги через точку с запятой: лекция; спорт.", buttons: []eventButton{f.button(d, "Пропустить", "skip")}}
}

func (f *EventEditingService) confirmEvent(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 5 || parts[0] != "admin" || parts[1] != "event_edit" || parts[2] != d.token || parts[3] != strconv.Itoa(int(d.step)) || parts[4] != "save" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.step != eventPreview {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	if d.updateField == domain.EventDates && !d.event.StartsAt.After(f.now()) {
		return f.reenterDates(ctx, userID, d)
	}
	id := d.event.ID
	err = f.Update(ctx, d.event, d.updateField)
	if errors.Is(err, domain.ErrEventStartInPast) {
		return f.reenterDates(ctx, userID, d)
	}
	if errors.Is(err, domain.ErrCapacityBelowRegistrations) {
		return f.invalid(d, "Количество мест не может быть меньше числа зарегистрированных участников."), nil
	}
	if errors.Is(err, domain.ErrEventNotFound) {
		f.clear(userID)
		return eventScreen{text: "Мероприятие уже удалено.", buttons: []eventButton{{"Мероприятия", "admin:events"}}}, nil
	}
	if err != nil {
		return f.invalid(d, "Не удалось сохранить мероприятие. Нажмите «Сохранить» ещё раз."), fmt.Errorf("save event: %w", err)
	}
	f.clear(userID)
	return eventScreen{text: fmt.Sprintf("Мероприятие «%s» (№%d) обновлено.", d.event.Title, id), buttons: []eventButton{{"Мероприятия", "admin:events"}}}, nil
}

func (f *EventEditingService) reenterDates(ctx context.Context, userID int64, d eventDraft) (eventScreen, error) {
	d.step = eventStart
	if err := f.save(ctx, userID, d); err != nil {
		return eventScreen{}, err
	}
	return f.invalid(d, "Время начала уже прошло. Введите новую дату и время."), nil
}

func (f *EventEditingService) previewPrompt(d eventDraft) eventScreen {
	capacity := "без ограничений"
	if d.event.Capacity != nil {
		capacity = strconv.Itoa(int(*d.event.Capacity))
	}
	return eventScreen{
		text:    fmt.Sprintf("Проверьте мероприятие:\n\n%s\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nУчастников: %s\nТеги: %s", d.event.Title, d.event.Description, d.event.Location, d.event.StartsAt.In(f.location).Format(eventDateLayout), d.event.EndsAt.In(f.location).Format(eventDateLayout), capacity, strings.Join(d.event.Tags, "; ")),
		photoID: d.event.PhotoFileID,
		buttons: []eventButton{f.button(d, "Сохранить", "save")},
	}
}

func (f *EventEditingService) beginEdit(ctx context.Context, userID int64, event domain.Event, field domain.EventField) (eventScreen, error) {
	steps := map[domain.EventField]eventStep{
		domain.EventTitle: eventTitle, domain.EventDescription: eventDescription,
		domain.EventPhoto: eventPhoto, domain.EventLocation: eventLocation,
		domain.EventDates: eventStart, domain.EventCapacity: eventCapacity, domain.EventTags: eventTags,
	}
	step, ok := steps[field]
	if !ok {
		return eventScreen{}, errors.New("unsupported event field")
	}
	if f.repo == nil {
		return eventScreen{}, errors.New("event service is not configured")
	}
	d := cloneDraft(eventDraft{event: event, step: step, token: rand.Text(), updateField: field})
	f.mu.Lock()
	if err := f.save(ctx, userID, d); err != nil {
		f.mu.Unlock()
		return eventScreen{}, err
	}
	view := f.prompt(d)
	f.mu.Unlock()
	return view, nil
}

func (f *EventEditingService) inState(userID int64, step eventStep) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, err := f.machine.CurrentState(context.Background(), userID)
	if err != nil {
		reportEditingStorageError(err)
		return false
	}
	return state == editingConversationStates[step]
}

func (f *EventEditingService) snapshot(ctx context.Context, userID int64) (*eventDraft, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	saved := cloneDraft(d)
	return &saved, nil
}

func (f *EventEditingService) restore(ctx context.Context, userID int64, saved eventDraft) (eventScreen, eventDraft, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := cloneDraft(saved)
	if err := f.save(ctx, userID, d); err != nil {
		return eventScreen{}, eventDraft{}, err
	}
	return f.prompt(d), d, nil
}

func (f *EventEditingService) staleCallback(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного редактирования мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 5 || parts[0] != "admin" || parts[1] != "event_edit" || parts[2] != d.token || parts[3] != strconv.Itoa(int(d.step)) {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	return f.prompt(d), nil
}

type Repository interface {
	Update(context.Context, domain.Event, domain.EventField) error
}

func (s *EventEditingService) Update(ctx context.Context, event domain.Event, field domain.EventField) error {
	if event.ID <= 0 {
		return fmt.Errorf("%w: missing event ID", domain.ErrInvalidEvent)
	}
	if err := s.validate(event, field); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, event, field); err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	return nil
}
func (s *EventEditingService) validate(event domain.Event, field domain.EventField) error {
	switch field {
	case domain.EventTitle:
		if strings.TrimSpace(event.Title) == "" {
			return fmt.Errorf("%w: title is required", domain.ErrInvalidEvent)
		}
	case domain.EventLocation:
		if strings.TrimSpace(event.Location) == "" {
			return fmt.Errorf("%w: location is required", domain.ErrInvalidEvent)
		}
	case domain.EventDates:
		if !event.StartsAt.After(s.now()) {
			return domain.ErrEventStartInPast
		}
		if !event.EndsAt.After(event.StartsAt) {
			return fmt.Errorf("%w: end must follow start", domain.ErrInvalidEvent)
		}
	case domain.EventCapacity:
		if event.Capacity != nil && *event.Capacity <= 0 {
			return fmt.Errorf("%w: capacity must be positive", domain.ErrInvalidEvent)
		}
	case domain.EventDescription, domain.EventPhoto, domain.EventTags:
	default:
		return fmt.Errorf("%w: unsupported field", domain.ErrInvalidEvent)
	}
	return nil
}
