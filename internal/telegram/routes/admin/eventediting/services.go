package eventediting

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
	eventTitle       fsm.StateID = "admin.event_editing.title"
	eventDescription fsm.StateID = "admin.event_editing.description"
	eventPhoto       fsm.StateID = "admin.event_editing.photo"
	eventLocation    fsm.StateID = "admin.event_editing.location"
	eventStart       fsm.StateID = "admin.event_editing.start"
	eventEnd         fsm.StateID = "admin.event_editing.end"
	eventCapacity    fsm.StateID = "admin.event_editing.capacity"
	eventTags        fsm.StateID = "admin.event_editing.tags"
	eventPreview     fsm.StateID = "admin.event_editing.preview"
)

const eventDateLayout = "02.01.2006 15:04"

type eventDraft struct {
	event       domain.Event
	updateField domain.EventField
	keyboards.TokenizedDraft
}

type editingResult struct {
	state   fsm.StateID
	draft   eventDraft
	message string
}

type EventEditingService struct {
	machine  *fsm.FSM
	repo     Repository
	location *time.Location
	now      func() time.Time
}

func NewService(machine *fsm.FSM, repository Repository) (*EventEditingService, error) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return nil, fmt.Errorf("load event timezone: %w", err)
	}
	flow := &EventEditingService{repo: repository, machine: machine, location: location, now: time.Now}
	return flow, nil
}

func cloneDraft(d eventDraft) eventDraft {
	d.event.Tags = slices.Clone(d.event.Tags)
	if d.event.Capacity != nil {
		capacity := *d.event.Capacity
		d.event.Capacity = &capacity
	}
	return d
}

func (f *EventEditingService) cancelForm(userID int64, token string) (editingResult, bool) {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}, false
	}
	if !d.IsTokenValid(token) {
		return editingResult{state: state, draft: d, message: "Эта кнопка устарела."}, false
	}

	f.machine.Clear(userID)
	return editingResult{state: fsm.Idle}, true
}

func (f *EventEditingService) setTitle(userID int64, text string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	if utf8.RuneCountInString(text) > 200 || strings.ContainsAny(text, "\r\n") {
		return editingResult{state: state, draft: d, message: "Название должно быть в одну строку, до 200 символов."}
	}
	d.event.Title = text
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) setDescription(userID int64, text string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	if utf8.RuneCountInString(text) > 2000 {
		return editingResult{state: state, draft: d, message: "Описание должно быть не длиннее 2000 символов."}
	}
	d.event.Description = text
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) skipDescription(userID int64, token string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	if !d.IsTokenValid(token) {
		return editingResult{state: state, draft: d, message: "Эта кнопка устарела."}
	}

	d.event.Description = ""
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) setPhoto(userID int64, photo string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}

	if photo == "" {
		return editingResult{state: state, draft: d, message: "Отправьте изображение как фото или нажмите «Пропустить»."}
	}
	d.event.PhotoFileID = photo
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) skipPhoto(userID int64, token string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	if !d.IsTokenValid(token) {
		return editingResult{state: state, draft: d, message: "Эта кнопка устарела."}
	}

	d.event.PhotoFileID = ""
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) setLocation(userID int64, text string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	if utf8.RuneCountInString(text) > 300 {
		return editingResult{state: state, draft: d, message: "Место должно быть не длиннее 300 символов."}
	}
	d.event.Location = text
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) setStart(userID int64, text string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return editingResult{state: state, draft: d, message: "Используйте формат ДД.ММ.ГГГГ ЧЧ:ММ."}
	}
	if !value.After(f.now()) {
		return editingResult{state: state, draft: d, message: "Начало должно быть в будущем."}
	}
	d.event.StartsAt = value.UTC()
	d = f.replaceDraft(userID, eventEnd, d)
	return editingResult{state: eventEnd, draft: d}
}

func (f *EventEditingService) setEnd(userID int64, text string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return editingResult{state: state, draft: d, message: "Используйте формат ДД.ММ.ГГГГ ЧЧ:ММ."}
	}
	if !value.After(d.event.StartsAt) {
		return editingResult{state: state, draft: d, message: "Окончание должно быть позже начала."}
	}
	d.event.EndsAt = value.UTC()
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) setCapacity(userID int64, text string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil || value <= 0 {
		return editingResult{state: state, draft: d, message: "Введите положительное целое число или нажмите «Без ограничений»."}
	}
	capacity := int32(value)
	d.event.Capacity = &capacity
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) unlimitedCapacity(userID int64, token string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	if !d.IsTokenValid(token) {
		return editingResult{state: state, draft: d, message: "Эта кнопка устарела."}
	}

	d.event.Capacity = nil
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) setTags(userID int64, text string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, message: "Отправьте текст для текущего шага."}
	}

	if utf8.RuneCountInString(text) > 400 {
		return editingResult{state: state, draft: d, message: "Список тегов должен быть не длиннее 400 символов."}
	}
	seen := make(map[string]bool)
	var tags []string
	for _, tag := range strings.Split(text, ";") {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		if utf8.RuneCountInString(tag) > 40 || strings.ContainsAny(tag, "\r\n") {
			return editingResult{state: state, draft: d, message: "Каждый тег должен быть в одну строку, до 40 символов."}
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	if len(tags) == 0 || len(tags) > 10 {
		return editingResult{state: state, draft: d, message: "Введите от 1 до 10 тегов или нажмите «Пропустить»."}
	}
	d.event.Tags = tags
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) skipTags(userID int64, token string) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	if !d.IsTokenValid(token) {
		return editingResult{state: state, draft: d, message: "Эта кнопка устарела."}
	}

	d.event.Tags = nil
	d = f.replaceDraft(userID, eventPreview, d)
	return editingResult{state: eventPreview, draft: d}
}

func (f *EventEditingService) confirmEvent(ctx context.Context, userID int64, token string) (editingResult, error) {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}, nil
	}
	if !d.IsTokenValid(token) {
		return editingResult{state: state, draft: d, message: "Эта кнопка устарела."}, nil
	}

	if d.updateField == domain.EventDates && !d.event.StartsAt.After(f.now()) {
		return f.reenterDates(userID, d), nil
	}
	err := f.Update(ctx, d.event, d.updateField)
	if errors.Is(err, domain.ErrEventStartInPast) {
		return f.reenterDates(userID, d), nil
	}
	if errors.Is(err, domain.ErrCapacityBelowRegistrations) {
		return editingResult{state: state, draft: d, message: "Количество мест не может быть меньше числа зарегистрированных участников."}, nil
	}
	if errors.Is(err, domain.ErrEventNotFound) {
		f.machine.Clear(userID)
		return editingResult{state: fsm.Idle}, err
	}
	if err != nil {
		return editingResult{state: state, draft: d, message: "Не удалось сохранить мероприятие. Нажмите «Сохранить» ещё раз."}, fmt.Errorf("save event: %w", err)
	}
	f.machine.Clear(userID)
	return editingResult{state: fsm.Idle, draft: d}, nil
}

func (f *EventEditingService) reenterDates(userID int64, d eventDraft) editingResult {
	state := eventStart
	d = f.replaceDraft(userID, state, d)
	return editingResult{state: state, draft: d, message: "Время начала уже прошло. Введите новую дату и время."}
}

func (f *EventEditingService) beginEdit(userID int64, event domain.Event, field domain.EventField) (editingResult, error) {
	steps := map[domain.EventField]fsm.StateID{
		domain.EventTitle: eventTitle, domain.EventDescription: eventDescription,
		domain.EventPhoto: eventPhoto, domain.EventLocation: eventLocation,
		domain.EventDates: eventStart, domain.EventCapacity: eventCapacity, domain.EventTags: eventTags,
	}
	step, ok := steps[field]
	if !ok {
		return editingResult{}, errors.New("unsupported event field")
	}
	if f.repo == nil {
		return editingResult{}, errors.New("event service is not configured")
	}
	d := f.replaceDraft(userID, step, cloneDraft(eventDraft{event: event, updateField: field}))
	return editingResult{state: step, draft: d}, nil
}

func (f *EventEditingService) restore(userID int64, state fsm.StateID, saved eventDraft) eventDraft {
	return f.replaceDraft(userID, state, cloneDraft(saved))
}

func (f *EventEditingService) replaceDraft(userID int64, state fsm.StateID, d eventDraft) eventDraft {
	d.RotateScreenToken()
	f.machine.Replace(userID, state, d)
	return d
}

func (f *EventEditingService) staleCallback(userID int64) editingResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{state: fsm.Idle}
	}
	return editingResult{state: state, draft: d, message: "Эта кнопка устарела."}
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
