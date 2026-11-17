package event_creation

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

const (
	eventTitle       fsm.StateID = "admin.events.title"
	eventDescription fsm.StateID = "admin.events.description"
	eventPhoto       fsm.StateID = "admin.events.photo"
	eventLocation    fsm.StateID = "admin.events.location"
	eventStart       fsm.StateID = "admin.events.start"
	eventEnd         fsm.StateID = "admin.events.end"
	eventCapacity    fsm.StateID = "admin.events.capacity"
	eventTags        fsm.StateID = "admin.events.tags"
	eventPreview     fsm.StateID = "admin.events.preview"
)

const eventDateLayout = "02.01.2006 15:04"

type eventDraft struct {
	event           domain.Event
	token           string
	returnToPreview bool
}

type conversation struct {
	eventDraft
	state fsm.StateID
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

type EventCreationService struct {
	mu       sync.Mutex
	machine  *fsm.FSM[string, eventDraft]
	repo     Repository
	location *time.Location
	now      func() time.Time
}

func NewService(repository Repository) (*EventCreationService, error) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return nil, fmt.Errorf("load event timezone: %w", err)
	}
	flow := &EventCreationService{repo: repository, machine: fsm.NewInMemoryFSM[string, eventDraft]("idle"), location: location, now: time.Now}
	return flow, nil
}

func (f *EventCreationService) Cancel(userID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clear(userID)
}

func (f *EventCreationService) HasDraft(userID int64) bool {
	ctx := context.Background()
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok, err := f.load(ctx, userID)
	if err != nil {
		reportStorageError(err)
		return false
	}
	return ok
}

func (f *EventCreationService) begin(userID int64) eventScreen {
	ctx := context.Background()
	f.mu.Lock()
	defer f.mu.Unlock()
	d := conversation{state: eventTitle, eventDraft: eventDraft{token: rand.Text()}}
	if err := f.save(ctx, userID, d); err != nil {
		reportStorageError(err)
		return eventScreen{text: "Не удалось начать создание мероприятия."}
	}
	return f.prompt(d)
}

func cloneDraft(d conversation) conversation {
	d.event.Tags = slices.Clone(d.event.Tags)
	if d.event.Capacity != nil {
		capacity := *d.event.Capacity
		d.event.Capacity = &capacity
	}
	return d
}

func reportStorageError(err error) {
	slog.Error("conversation storage failed", "error", err)
}

func (f *EventCreationService) load(ctx context.Context, userID int64) (conversation, bool, error) {
	state, err := f.machine.CurrentState(ctx, userID)
	if err != nil {
		return conversation{}, false, err
	}
	if state == "idle" {
		return conversation{}, false, nil
	}
	switch state {
	case eventTitle, eventDescription, eventPhoto, eventLocation, eventStart, eventEnd, eventCapacity, eventTags, eventPreview:
	default:
		return conversation{}, false, fmt.Errorf("unexpected conversation state %q", state)
	}
	value, err := f.machine.GetData(ctx, userID, "session")
	if errors.Is(err, fsm.ErrUserDataNotFound) {
		return conversation{}, false, nil
	}
	if err != nil {
		return conversation{}, false, err
	}
	return conversation{eventDraft: value, state: state}, true, nil
}
func (f *EventCreationService) save(ctx context.Context, userID int64, value conversation) error {
	switch value.state {
	case eventTitle, eventDescription, eventPhoto, eventLocation, eventStart, eventEnd, eventCapacity, eventTags, eventPreview:
	default:
		return fmt.Errorf("invalid conversation state %q", value.state)
	}
	if err := f.machine.SetData(ctx, userID, "session", value.eventDraft); err != nil {
		return err
	}
	return f.machine.Transition(ctx, userID, value.state)
}
func (f *EventCreationService) clear(userID int64) {
	if err := f.machine.Finish(context.Background(), userID); err != nil {
		reportStorageError(err)
	}
}

func (f *EventCreationService) completeStep(ctx context.Context, userID int64, d conversation) (eventScreen, error) {
	d.token = rand.Text()
	previous := d.state
	switch previous {
	case eventTitle:
		d.state = eventDescription
	case eventDescription:
		d.state = eventPhoto
	case eventPhoto:
		d.state = eventLocation
	case eventLocation:
		d.state = eventStart
	case eventStart:
		d.state = eventEnd
	case eventEnd:
		d.state = eventCapacity
	case eventCapacity:
		d.state = eventTags
	case eventTags:
		d.state = eventPreview
	}
	if previous == eventEnd && d.returnToPreview {
		d.state, d.returnToPreview = eventPreview, false
	}
	if err := f.save(ctx, userID, d); err != nil {
		return eventScreen{}, err
	}
	return f.prompt(d), nil
}

func (f *EventCreationService) cancelForm(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 || parts[0] != "admin" || parts[1] != "event" || parts[2] != d.token || parts[3] != "cancel" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}

	f.clear(userID)
	return eventScreen{text: "Создание мероприятия отменено."}, nil
}

func (f *EventCreationService) button(d conversation, text, action string) eventButton {
	return eventButton{text, fmt.Sprintf("admin:event:%s:%s", d.token, action)}
}

func (f *EventCreationService) prompt(d conversation) eventScreen {
	var view eventScreen
	switch d.state {
	case eventTitle:
		view = eventScreen{text: "Создание мероприятия\nВведите название."}
	case eventDescription:
		view = f.descriptionPrompt(d)
	case eventPhoto:
		view = f.photoPrompt(d)
	case eventLocation:
		view = eventScreen{text: "Введите место проведения (адрес, аудитория)."}
	case eventStart:
		view = f.startPrompt()
	case eventEnd:
		view = f.endPrompt(d)
	case eventCapacity:
		view = f.capacityPrompt(d)
	case eventTags:
		view = f.tagsPrompt(d)
	case eventPreview:
		view = f.previewPrompt(d)
	}
	cancelText := "Отменить создание"

	view.buttons = append(view.buttons, f.button(d, cancelText, "cancel"))
	return view
}

func (f *EventCreationService) invalid(d conversation, message string) eventScreen {
	view := f.prompt(d)
	view.text = message + "\n\n" + view.text
	return view
}

func (f *EventCreationService) setTitle(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	if d.state != eventTitle {
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

func (f *EventCreationService) setDescription(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	if d.state != eventDescription {
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

func (f *EventCreationService) skipDescription(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 || parts[0] != "admin" || parts[1] != "event" || parts[2] != d.token || parts[3] != "skip" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.state != eventDescription {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	d.event.Description = ""
	return f.completeStep(ctx, userID, d)
}

func (f *EventCreationService) descriptionPrompt(d conversation) eventScreen {
	return eventScreen{text: "Введите описание мероприятия.", buttons: []eventButton{f.button(d, "Пропустить", "skip")}}
}

func (f *EventCreationService) setPhoto(ctx context.Context, userID int64, photo string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	if d.state != eventPhoto {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	if photo == "" {
		return f.invalid(d, "Отправьте изображение как фото или нажмите «Пропустить»."), nil
	}
	d.event.PhotoFileID = photo
	return f.completeStep(ctx, userID, d)
}

func (f *EventCreationService) skipPhoto(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 || parts[0] != "admin" || parts[1] != "event" || parts[2] != d.token || parts[3] != "skip" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.state != eventPhoto {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	d.event.PhotoFileID = ""
	return f.completeStep(ctx, userID, d)
}

func (f *EventCreationService) photoPrompt(d conversation) eventScreen {
	return eventScreen{text: "Отправьте фото мероприятия.", buttons: []eventButton{f.button(d, "Пропустить", "skip")}}
}

func (f *EventCreationService) setLocation(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	if d.state != eventLocation {
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

func (f *EventCreationService) setStart(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	if d.state != eventStart {
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

func (f *EventCreationService) setEnd(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	if d.state != eventEnd {
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

func (f *EventCreationService) startPrompt() eventScreen {
	example := f.now().In(f.location).Truncate(time.Minute).Add(time.Minute)
	return eventScreen{text: "Введите дату и время начала: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)}
}

func (f *EventCreationService) endPrompt(d conversation) eventScreen {
	example := d.event.StartsAt.In(f.location).Add(time.Hour)
	return eventScreen{text: "Введите дату и время окончания: ДД.ММ.ГГГГ ЧЧ:ММ.\nНапример: " + example.Format(eventDateLayout)}
}

func (f *EventCreationService) setCapacity(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	if d.state != eventCapacity {
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

func (f *EventCreationService) unlimitedCapacity(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 || parts[0] != "admin" || parts[1] != "event" || parts[2] != d.token || parts[3] != "skip" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.state != eventCapacity {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	d.event.Capacity = nil
	return f.completeStep(ctx, userID, d)
}

func (f *EventCreationService) capacityPrompt(d conversation) eventScreen {
	return eventScreen{text: "Введите максимальное количество участников.", buttons: []eventButton{f.button(d, "Без ограничений", "skip")}}
}

func (f *EventCreationService) setTags(ctx context.Context, userID int64, text string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	if d.state != eventTags {
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

func (f *EventCreationService) skipTags(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 || parts[0] != "admin" || parts[1] != "event" || parts[2] != d.token || parts[3] != "skip" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.state != eventTags {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	d.event.Tags = nil
	return f.completeStep(ctx, userID, d)
}

func (f *EventCreationService) tagsPrompt(d conversation) eventScreen {
	return eventScreen{text: "Введите теги через точку с запятой: лекция; спорт.", buttons: []eventButton{f.button(d, "Пропустить", "skip")}}
}

func (f *EventCreationService) confirmEvent(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 || parts[0] != "admin" || parts[1] != "event" || parts[2] != d.token || parts[3] != "save" {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	if d.state != eventPreview {
		return f.invalid(d, "Этот шаг уже завершён."), nil
	}

	if !d.event.StartsAt.After(f.now()) {
		return f.reenterDates(ctx, userID, d)
	}
	id, err := f.Create(ctx, d.event)
	if errors.Is(err, domain.ErrEventStartInPast) {
		return f.reenterDates(ctx, userID, d)
	}

	if err != nil {
		return f.invalid(d, "Не удалось сохранить мероприятие. Нажмите «Сохранить» ещё раз."), fmt.Errorf("save event: %w", err)
	}
	f.clear(userID)
	return eventScreen{text: fmt.Sprintf("Мероприятие «%s» (№%d) создано.", d.event.Title, id)}, nil
}

func (f *EventCreationService) reenterDates(ctx context.Context, userID int64, d conversation) (eventScreen, error) {
	d.token = rand.Text()
	d.state, d.returnToPreview = eventStart, true
	if err := f.save(ctx, userID, d); err != nil {
		return eventScreen{}, err
	}
	return f.invalid(d, "Время начала уже прошло. Введите новую дату и время."), nil
}

func (f *EventCreationService) previewPrompt(d conversation) eventScreen {
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

func (f *EventCreationService) inState(userID int64, step fsm.StateID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, err := f.machine.CurrentState(context.Background(), userID)
	if err != nil {
		reportStorageError(err)
		return false
	}
	return state == step
}

func (f *EventCreationService) snapshot(ctx context.Context, userID int64) (*conversation, error) {
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

func (f *EventCreationService) restore(ctx context.Context, userID int64, saved conversation) (eventScreen, conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := cloneDraft(saved)
	if err := f.save(ctx, userID, d); err != nil {
		return eventScreen{}, conversation{}, err
	}
	return f.prompt(d), d, nil
}

func (f *EventCreationService) staleCallback(ctx context.Context, userID int64, callback string) (eventScreen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, exists, err := f.load(ctx, userID)
	if err != nil {
		return eventScreen{}, err
	}
	if !exists {
		return eventScreen{text: "Нет активного создания мероприятия."}, nil
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 || parts[0] != "admin" || parts[1] != "event" || parts[2] != d.token {
		return f.invalid(d, "Эта кнопка устарела."), nil
	}
	return f.prompt(d), nil
}

type Repository interface {
	Create(context.Context, domain.Event) (int64, error)
}

func (s *EventCreationService) Create(ctx context.Context, event domain.Event) (int64, error) {
	for _, field := range []domain.EventField{domain.EventTitle, domain.EventLocation, domain.EventDates, domain.EventCapacity} {
		if err := s.validate(event, field); err != nil {
			return 0, err
		}
	}
	id, err := s.repo.Create(ctx, event)
	if err != nil {
		return 0, fmt.Errorf("create event: %w", err)
	}
	return id, nil
}
func (s *EventCreationService) validate(event domain.Event, field domain.EventField) error {
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
