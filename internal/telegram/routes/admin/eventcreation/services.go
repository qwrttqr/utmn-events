package eventcreation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
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
	returnToPreview bool
	keyboards.TokenizedDraft
}

type creationResult struct {
	state   fsm.StateID
	draft   eventDraft
	message string
	eventID int64
}

type EventCreationService struct {
	machine  *fsm.FSM
	repo     Repository
	location *time.Location
	now      func() time.Time
}

func NewService(machine *fsm.FSM, repository Repository) (*EventCreationService, error) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return nil, fmt.Errorf("load event timezone: %w", err)
	}
	flow := &EventCreationService{repo: repository, machine: machine, location: location, now: time.Now}
	return flow, nil
}

func (f *EventCreationService) begin(userID int64) creationResult {
	d := f.replaceDraft(userID, eventTitle, eventDraft{})
	return creationResult{state: eventTitle, draft: d}
}

func cloneDraft(d eventDraft) eventDraft {
	d.event.Tags = slices.Clone(d.event.Tags)
	if d.event.Capacity != nil {
		capacity := *d.event.Capacity
		d.event.Capacity = &capacity
	}
	return d
}

func (f *EventCreationService) cancelForm(userID int64, token string) (creationResult, bool) {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}, false
	}
	if !d.IsTokenValid(token) {
		return creationResult{state: state, draft: d, message: "Эта кнопка устарела."}, false
	}

	f.machine.Clear(userID)
	return creationResult{state: fsm.Idle}, true
}

func (f *EventCreationService) setTitle(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return creationResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	if utf8.RuneCountInString(text) > 200 || strings.ContainsAny(text, "\r\n") {
		return creationResult{state: state, draft: d, message: "Название должно быть в одну строку, до 200 символов."}
	}
	d.event.Title = text
	d = f.replaceDraft(userID, eventDescription, d)
	return creationResult{state: eventDescription, draft: d}
}

func (f *EventCreationService) setDescription(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	if utf8.RuneCountInString(text) > 2000 {
		return creationResult{state: state, draft: d, message: "Описание должно быть не длиннее 2000 символов."}
	}
	d.event.Description = text
	d = f.replaceDraft(userID, eventPhoto, d)
	return creationResult{state: eventPhoto, draft: d}
}

func (f *EventCreationService) skipDescription(userID int64, token string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	if !d.IsTokenValid(token) {
		return creationResult{state: state, draft: d, message: "Эта кнопка устарела."}
	}

	d.event.Description = ""
	d = f.replaceDraft(userID, eventPhoto, d)
	return creationResult{state: eventPhoto, draft: d}
}

func (f *EventCreationService) setPhoto(userID int64, photo string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	if photo == "" {
		return creationResult{state: state, draft: d, message: "Отправьте изображение как фото или нажмите «Пропустить»."}
	}
	d.event.PhotoFileID = photo
	d = f.replaceDraft(userID, eventLocation, d)
	return creationResult{state: eventLocation, draft: d}
}

func (f *EventCreationService) skipPhoto(userID int64, token string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	if !d.IsTokenValid(token) {
		return creationResult{state: state, draft: d, message: "Эта кнопка устарела."}
	}

	d.event.PhotoFileID = ""
	d = f.replaceDraft(userID, eventLocation, d)
	return creationResult{state: eventLocation, draft: d}
}

func (f *EventCreationService) setLocation(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	if utf8.RuneCountInString(text) > 300 {
		return creationResult{state: state, draft: d, message: "Место должно быть не длиннее 300 символов."}
	}
	d.event.Location = text
	d = f.replaceDraft(userID, eventStart, d)
	return creationResult{state: eventStart, draft: d}
}

func (f *EventCreationService) setStart(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return creationResult{state: state, draft: d, message: "Используйте формат ДД.ММ.ГГГГ ЧЧ:ММ."}
	}
	if !value.After(f.now()) {
		return creationResult{state: state, draft: d, message: "Начало должно быть в будущем."}
	}
	d.event.StartsAt = value.UTC()
	d = f.replaceDraft(userID, eventEnd, d)
	return creationResult{state: eventEnd, draft: d}
}

func (f *EventCreationService) setEnd(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return creationResult{state: state, draft: d, message: "Используйте формат ДД.ММ.ГГГГ ЧЧ:ММ."}
	}
	if !value.After(d.event.StartsAt) {
		return creationResult{state: state, draft: d, message: "Окончание должно быть позже начала."}
	}
	d.event.EndsAt = value.UTC()
	next := eventCapacity
	if d.returnToPreview {
		next = eventPreview
		d.returnToPreview = false
	}
	d = f.replaceDraft(userID, next, d)
	return creationResult{state: next, draft: d}
}

func (f *EventCreationService) setCapacity(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil || value <= 0 {
		return creationResult{state: state, draft: d, message: "Введите положительное целое число или нажмите «Без ограничений»."}
	}
	capacity := int32(value)
	d.event.Capacity = &capacity
	d = f.replaceDraft(userID, eventTags, d)
	return creationResult{state: eventTags, draft: d}
}

func (f *EventCreationService) unlimitedCapacity(userID int64, token string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	if !d.IsTokenValid(token) {
		return creationResult{state: state, draft: d, message: "Эта кнопка устарела."}
	}

	d.event.Capacity = nil
	d = f.replaceDraft(userID, eventTags, d)
	return creationResult{state: eventTags, draft: d}
}

func (f *EventCreationService) setTags(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	if utf8.RuneCountInString(text) > 400 {
		return creationResult{state: state, draft: d, message: "Список тегов должен быть не длиннее 400 символов."}
	}
	seen := make(map[string]bool)
	var tags []string
	for _, tag := range strings.Split(text, ";") {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		if utf8.RuneCountInString(tag) > 40 || strings.ContainsAny(tag, "\r\n") {
			return creationResult{state: state, draft: d, message: "Каждый тег должен быть в одну строку, до 40 символов."}
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	if len(tags) == 0 || len(tags) > 10 {
		return creationResult{state: state, draft: d, message: "Введите от 1 до 10 тегов или нажмите «Пропустить»."}
	}
	d.event.Tags = tags
	d = f.replaceDraft(userID, eventPreview, d)
	return creationResult{state: eventPreview, draft: d}
}

func (f *EventCreationService) skipTags(userID int64, token string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	if !d.IsTokenValid(token) {
		return creationResult{state: state, draft: d, message: "Эта кнопка устарела."}
	}

	d.event.Tags = nil
	d = f.replaceDraft(userID, eventPreview, d)
	return creationResult{state: eventPreview, draft: d}
}

func (f *EventCreationService) confirmEvent(ctx context.Context, userID int64, token string) (creationResult, error) {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}, nil
	}
	if !d.IsTokenValid(token) {
		return creationResult{state: state, draft: d, message: "Эта кнопка устарела."}, nil
	}

	if !d.event.StartsAt.After(f.now()) {
		return f.reenterDates(userID, d), nil
	}
	id, err := f.Create(ctx, d.event)
	if errors.Is(err, domain.ErrEventStartInPast) {
		return f.reenterDates(userID, d), nil
	}

	if err != nil {
		return creationResult{state: state, draft: d, message: "Не удалось сохранить мероприятие. Нажмите «Сохранить» ещё раз."}, fmt.Errorf("save event: %w", err)
	}
	f.machine.Clear(userID)
	return creationResult{state: fsm.Idle, draft: d, eventID: id}, nil
}

func (f *EventCreationService) reenterDates(userID int64, d eventDraft) creationResult {
	state := eventStart
	d.returnToPreview = true
	d = f.replaceDraft(userID, state, d)
	return creationResult{state: state, draft: d, message: "Время начала уже прошло. Введите новую дату и время."}
}

func (f *EventCreationService) replaceDraft(userID int64, state fsm.StateID, d eventDraft) eventDraft {
	d.RotateScreenToken()
	f.machine.Replace(userID, state, d)
	return d
}

func (f *EventCreationService) staleCallback(userID int64) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{state: fsm.Idle}
	}
	return creationResult{state: state, draft: d, message: "Эта кнопка устарела."}
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
