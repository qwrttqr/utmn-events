package event_editing

import (
	"context"
	"errors"
	"fmt"
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
}

type editingStatus uint8

const (
	stepStarted editingStatus = iota + 1
	noActiveDraft
	editingCancelled
	eventUpdated
	eventMissing
	stepAlreadyCompleted
	textRequired
	invalidTitle
	descriptionTooLong
	photoRequired
	locationTooLong
	invalidDateFormat
	startMustBeFuture
	endMustFollowStart
	invalidCapacity
	tagsTooLong
	invalidTag
	invalidTagCount
	saveFailed
	startExpired
	staleAction
	capacityBelowRegistrations
)

type editingResult struct {
	state  fsm.StateID
	draft  eventDraft
	status editingStatus
}

type EventEditingService struct {
	mu       sync.Mutex
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

func (f *EventEditingService) completeStep(userID int64, state fsm.StateID, d eventDraft) editingResult {
	if state == eventStart {
		state = eventEnd
	} else {
		state = eventPreview
	}
	f.machine.Replace(userID, state, d)
	return editingResult{state: state, draft: d, status: stepStarted}
}

func (f *EventEditingService) cancelForm(userID int64) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, _, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}

	f.machine.Clear(userID)
	return editingResult{status: editingCancelled}
}

func (f *EventEditingService) setTitle(userID int64, text string) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventTitle {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, status: textRequired}
	}

	if utf8.RuneCountInString(text) > 200 || strings.ContainsAny(text, "\r\n") {
		return editingResult{state: state, draft: d, status: invalidTitle}
	}
	d.event.Title = text
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) setDescription(userID int64, text string) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventDescription {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, status: textRequired}
	}

	if utf8.RuneCountInString(text) > 2000 {
		return editingResult{state: state, draft: d, status: descriptionTooLong}
	}
	d.event.Description = text
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) skipDescription(userID int64) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventDescription {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	d.event.Description = ""
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) setPhoto(userID int64, photo string) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventPhoto {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	if photo == "" {
		return editingResult{state: state, draft: d, status: photoRequired}
	}
	d.event.PhotoFileID = photo
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) skipPhoto(userID int64) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventPhoto {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	d.event.PhotoFileID = ""
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) setLocation(userID int64, text string) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventLocation {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, status: textRequired}
	}

	if utf8.RuneCountInString(text) > 300 {
		return editingResult{state: state, draft: d, status: locationTooLong}
	}
	d.event.Location = text
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) setStart(userID int64, text string) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventStart {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, status: textRequired}
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return editingResult{state: state, draft: d, status: invalidDateFormat}
	}
	if !value.After(f.now()) {
		return editingResult{state: state, draft: d, status: startMustBeFuture}
	}
	d.event.StartsAt = value.UTC()
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) setEnd(userID int64, text string) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventEnd {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, status: textRequired}
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return editingResult{state: state, draft: d, status: invalidDateFormat}
	}
	if !value.After(d.event.StartsAt) {
		return editingResult{state: state, draft: d, status: endMustFollowStart}
	}
	d.event.EndsAt = value.UTC()
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) setCapacity(userID int64, text string) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventCapacity {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, status: textRequired}
	}

	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil || value <= 0 {
		return editingResult{state: state, draft: d, status: invalidCapacity}
	}
	capacity := int32(value)
	d.event.Capacity = &capacity
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) unlimitedCapacity(userID int64) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventCapacity {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	d.event.Capacity = nil
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) setTags(userID int64, text string) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventTags {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return editingResult{state: state, draft: d, status: textRequired}
	}

	if utf8.RuneCountInString(text) > 400 {
		return editingResult{state: state, draft: d, status: tagsTooLong}
	}
	seen := make(map[string]bool)
	var tags []string
	for _, tag := range strings.Split(text, ";") {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		if utf8.RuneCountInString(tag) > 40 || strings.ContainsAny(tag, "\r\n") {
			return editingResult{state: state, draft: d, status: invalidTag}
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	if len(tags) == 0 || len(tags) > 10 {
		return editingResult{state: state, draft: d, status: invalidTagCount}
	}
	d.event.Tags = tags
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) skipTags(userID int64) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	if state != eventTags {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	d.event.Tags = nil
	return f.completeStep(userID, state, d)
}

func (f *EventEditingService) confirmEvent(ctx context.Context, userID int64) (editingResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}, nil
	}
	if state != eventPreview {
		return editingResult{state: state, draft: d, status: stepAlreadyCompleted}, nil
	}

	if d.updateField == domain.EventDates && !d.event.StartsAt.After(f.now()) {
		return f.reenterDates(userID, d), nil
	}
	err := f.Update(ctx, d.event, d.updateField)
	if errors.Is(err, domain.ErrEventStartInPast) {
		return f.reenterDates(userID, d), nil
	}
	if errors.Is(err, domain.ErrCapacityBelowRegistrations) {
		return editingResult{state: state, draft: d, status: capacityBelowRegistrations}, nil
	}
	if errors.Is(err, domain.ErrEventNotFound) {
		f.machine.Clear(userID)
		return editingResult{status: eventMissing}, nil
	}
	if err != nil {
		return editingResult{state: state, draft: d, status: saveFailed}, fmt.Errorf("save event: %w", err)
	}
	f.machine.Clear(userID)
	return editingResult{status: eventUpdated, draft: d}, nil
}

func (f *EventEditingService) reenterDates(userID int64, d eventDraft) editingResult {
	state := eventStart
	f.machine.Replace(userID, state, d)
	return editingResult{state: state, draft: d, status: startExpired}
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
	d := cloneDraft(eventDraft{event: event, updateField: field})
	f.mu.Lock()
	f.machine.Replace(userID, step, d)
	f.mu.Unlock()
	return editingResult{state: step, draft: d, status: stepStarted}, nil
}

func (f *EventEditingService) inState(userID int64, step fsm.StateID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.machine.CurrentState(userID) == step
}

func (f *EventEditingService) restore(userID int64, state fsm.StateID, saved eventDraft) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.machine.Replace(userID, state, cloneDraft(saved))
}

func (f *EventEditingService) staleCallback(userID int64) editingResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, d, exists := fsm.Load[eventDraft](f.machine, userID)
	if !exists {
		return editingResult{status: noActiveDraft}
	}
	return editingResult{state: state, draft: d, status: staleAction}
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
