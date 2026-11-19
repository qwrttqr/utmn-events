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
}

type creationStatus uint8

const (
	stepStarted creationStatus = iota + 1
	noActiveDraft
	creationCancelled
	eventCreated
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
)

type creationResult struct {
	state   fsm.StateID
	draft   eventDraft
	status  creationStatus
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
	d := eventDraft{}
	f.machine.Replace(userID, eventTitle, d)
	return creationResult{state: eventTitle, draft: d, status: stepStarted}
}

func cloneDraft(d eventDraft) eventDraft {
	d.event.Tags = slices.Clone(d.event.Tags)
	if d.event.Capacity != nil {
		capacity := *d.event.Capacity
		d.event.Capacity = &capacity
	}
	return d
}

func (f *EventCreationService) completeStep(userID int64, state fsm.StateID, d eventDraft) creationResult {
	previous := state
	switch previous {
	case eventTitle:
		state = eventDescription
	case eventDescription:
		state = eventPhoto
	case eventPhoto:
		state = eventLocation
	case eventLocation:
		state = eventStart
	case eventStart:
		state = eventEnd
	case eventEnd:
		state = eventCapacity
	case eventCapacity:
		state = eventTags
	case eventTags:
		state = eventPreview
	}
	if previous == eventEnd && d.returnToPreview {
		state, d.returnToPreview = eventPreview, false
	}
	f.machine.Replace(userID, state, d)
	return creationResult{state: state, draft: d, status: stepStarted}
}

func (f *EventCreationService) cancelForm(userID int64) creationResult {
	_, _, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}

	f.machine.Clear(userID)
	return creationResult{status: creationCancelled}
}

func (f *EventCreationService) setTitle(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventTitle {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, status: textRequired}
	}

	if utf8.RuneCountInString(text) > 200 || strings.ContainsAny(text, "\r\n") {
		return creationResult{state: state, draft: d, status: invalidTitle}
	}
	d.event.Title = text
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) setDescription(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventDescription {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, status: textRequired}
	}

	if utf8.RuneCountInString(text) > 2000 {
		return creationResult{state: state, draft: d, status: descriptionTooLong}
	}
	d.event.Description = text
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) skipDescription(userID int64) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventDescription {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	d.event.Description = ""
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) setPhoto(userID int64, photo string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventPhoto {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	if photo == "" {
		return creationResult{state: state, draft: d, status: photoRequired}
	}
	d.event.PhotoFileID = photo
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) skipPhoto(userID int64) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventPhoto {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	d.event.PhotoFileID = ""
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) setLocation(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventLocation {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, status: textRequired}
	}

	if utf8.RuneCountInString(text) > 300 {
		return creationResult{state: state, draft: d, status: locationTooLong}
	}
	d.event.Location = text
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) setStart(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventStart {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, status: textRequired}
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return creationResult{state: state, draft: d, status: invalidDateFormat}
	}
	if !value.After(f.now()) {
		return creationResult{state: state, draft: d, status: startMustBeFuture}
	}
	d.event.StartsAt = value.UTC()
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) setEnd(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventEnd {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, status: textRequired}
	}

	value, err := time.ParseInLocation(eventDateLayout, text, f.location)
	if err != nil {
		return creationResult{state: state, draft: d, status: invalidDateFormat}
	}
	if !value.After(d.event.StartsAt) {
		return creationResult{state: state, draft: d, status: endMustFollowStart}
	}
	d.event.EndsAt = value.UTC()
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) setCapacity(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventCapacity {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, status: textRequired}
	}

	value, err := strconv.ParseInt(text, 10, 32)
	if err != nil || value <= 0 {
		return creationResult{state: state, draft: d, status: invalidCapacity}
	}
	capacity := int32(value)
	d.event.Capacity = &capacity
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) unlimitedCapacity(userID int64) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventCapacity {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	d.event.Capacity = nil
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) setTags(userID int64, text string) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventTags {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return creationResult{state: state, draft: d, status: textRequired}
	}

	if utf8.RuneCountInString(text) > 400 {
		return creationResult{state: state, draft: d, status: tagsTooLong}
	}
	seen := make(map[string]bool)
	var tags []string
	for _, tag := range strings.Split(text, ";") {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" || seen[tag] {
			continue
		}
		if utf8.RuneCountInString(tag) > 40 || strings.ContainsAny(tag, "\r\n") {
			return creationResult{state: state, draft: d, status: invalidTag}
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	if len(tags) == 0 || len(tags) > 10 {
		return creationResult{state: state, draft: d, status: invalidTagCount}
	}
	d.event.Tags = tags
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) skipTags(userID int64) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	if state != eventTags {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}
	}

	d.event.Tags = nil
	return f.completeStep(userID, state, d)
}

func (f *EventCreationService) confirmEvent(ctx context.Context, userID int64) (creationResult, error) {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}, nil
	}
	if state != eventPreview {
		return creationResult{state: state, draft: d, status: stepAlreadyCompleted}, nil
	}

	if !d.event.StartsAt.After(f.now()) {
		return f.reenterDates(userID, d), nil
	}
	id, err := f.Create(ctx, d.event)
	if errors.Is(err, domain.ErrEventStartInPast) {
		return f.reenterDates(userID, d), nil
	}

	if err != nil {
		return creationResult{state: state, draft: d, status: saveFailed}, fmt.Errorf("save event: %w", err)
	}
	f.machine.Clear(userID)
	return creationResult{status: eventCreated, draft: d, eventID: id}, nil
}

func (f *EventCreationService) reenterDates(userID int64, d eventDraft) creationResult {
	state := eventStart
	d.returnToPreview = true
	f.machine.Replace(userID, state, d)
	return creationResult{state: state, draft: d, status: startExpired}
}

func (f *EventCreationService) inState(userID int64, step fsm.StateID) bool {
	return f.machine.CurrentState(userID) == step
}

func (f *EventCreationService) restore(userID int64, state fsm.StateID, saved eventDraft) {
	f.machine.Replace(userID, state, cloneDraft(saved))
}

func (f *EventCreationService) staleCallback(userID int64) creationResult {
	state, d, exists := fsm.GetDraft[eventDraft](f.machine, userID)
	if !exists {
		return creationResult{status: noActiveDraft}
	}
	return creationResult{state: state, draft: d, status: staleAction}
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
