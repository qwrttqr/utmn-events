package eventmanagement

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	_ "time/tzdata"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/pagination"
)

type Repository interface {
	List(context.Context, int, int) ([]domain.EventSummary, error)
	Get(context.Context, int64) (domain.Event, error)
	Delete(context.Context, int64) error
}

type UserRepository interface {
	ListForEvent(context.Context, int64, int, int) ([]domain.User, error)
}

const (
	list          fsm.StateID = "admin.events.list"
	card          fsm.StateID = "admin.events.card"
	fields        fsm.StateID = "admin.events.fields"
	confirmDelete fsm.StateID = "admin.events.delete"
	participants  fsm.StateID = "admin.events.participants"
)
const pageSize = pagination.Size

type session struct {
	page    int
	eventID int64
}

type managementStatus uint8

const (
	managementExpired managementStatus = iota + 1
	managementDeleted
)

type managementResult struct {
	state    fsm.StateID
	session  session
	status   managementStatus
	event    domain.Event
	events   []domain.EventSummary
	users    []domain.User
	more     bool
	location *time.Location
}

type editRequest struct {
	event domain.Event
	field domain.EventField
}

type EventManagementService struct {
	machine  *fsm.FSM
	repo     Repository
	users    UserRepository
	adminIDs []int64
}

func NewService(
	machine *fsm.FSM,
	repo Repository,
	users UserRepository,
	adminIDs []int64,
) *EventManagementService {
	return &EventManagementService{
		machine:  machine,
		repo:     repo,
		users:    users,
		adminIDs: slices.Clone(adminIDs),
	}
}
func (f *EventManagementService) Cancel(id int64) {
	switch f.machine.CurrentState(id) {
	case list, card, fields, confirmDelete, participants:
		f.machine.Clear(id)
	}
}

func (f *EventManagementService) process(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	if callback == "admin:events" {
		return f.list(ctx, userID)
	}
	_, action, ok := parseCallback(callback)
	if !ok {
		return managementResult{status: managementExpired}, nil, nil
	}
	if strings.HasPrefix(action, "page") {
		return f.page(ctx, userID, callback)
	}
	if action == "open" {
		return f.open(ctx, userID, callback)
	}
	if action == "edit" {
		return f.edit(ctx, userID, callback)
	}
	if action == "participants" {
		return f.participants(ctx, userID, callback)
	}
	if strings.HasPrefix(action, "field") {
		return f.field(ctx, userID, callback)
	}
	if action == "delete" {
		return f.delete(ctx, userID, callback)
	}
	if action == "keep" {
		return f.keep(ctx, userID, callback)
	}
	if action == "confirm" {
		return f.confirmDelete(ctx, userID, callback)
	}
	return managementResult{status: managementExpired}, nil, nil
}

func (f *EventManagementService) page(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	listPage := state == list && eventID == 0
	participantsPage := state == participants && eventID > 0 && eventID == s.eventID
	if !exists || !valid || !strings.HasPrefix(action, "page") || (!listPage && !participantsPage) {
		return managementResult{status: managementExpired}, nil, nil
	}

	if nextPage, ok := pagination.Move(s.page, action); ok {
		s.page = nextPage
	}

	return f.loadAndSave(ctx, userID, state, s)
}

func (f *EventManagementService) open(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || state != list || action != "open" || eventID == 0 {
		return managementResult{status: managementExpired}, nil, nil
	}

	s.eventID, state = eventID, card

	return f.loadAndSave(ctx, userID, state, s)
}

func (f *EventManagementService) edit(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || state != card || action != "edit" || eventID <= 0 || eventID != s.eventID {
		return managementResult{status: managementExpired}, nil, nil
	}

	state = fields

	return f.loadAndSave(ctx, userID, state, s)
}

func (f *EventManagementService) participants(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || state != card || action != "participants" || eventID <= 0 || eventID != s.eventID {
		return managementResult{status: managementExpired}, nil, nil
	}

	state, s.page = participants, 0

	return f.loadAndSave(ctx, userID, state, s)
}

func (f *EventManagementService) field(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || state != fields || !strings.HasPrefix(action, "field") || eventID <= 0 || eventID != s.eventID {
		return managementResult{status: managementExpired}, nil, nil
	}

	field := domain.EventField(strings.TrimPrefix(action, "field"))
	if !validField(field) {
		return managementResult{}, nil, errors.New("invalid event field")
	}
	event, err := f.repo.Get(ctx, eventID)
	if err != nil {
		return managementResult{}, nil, err
	}
	return managementResult{}, &editRequest{event, field}, nil
}

func (f *EventManagementService) delete(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || state != card || action != "delete" || eventID <= 0 || eventID != s.eventID {
		return managementResult{status: managementExpired}, nil, nil
	}

	state = confirmDelete

	return f.loadAndSave(ctx, userID, state, s)
}

func (f *EventManagementService) keep(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || state != confirmDelete || action != "keep" || eventID <= 0 || eventID != s.eventID {
		return managementResult{status: managementExpired}, nil, nil
	}

	state = card

	return f.loadAndSave(ctx, userID, state, s)
}

func (f *EventManagementService) confirmDelete(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || state != confirmDelete || action != "confirm" || eventID <= 0 || eventID != s.eventID {
		return managementResult{status: managementExpired}, nil, nil
	}

	if err := f.repo.Delete(ctx, eventID); err != nil {
		return managementResult{}, nil, err
	}
	f.machine.Clear(userID)
	return managementResult{status: managementDeleted}, nil, nil
}

func (f *EventManagementService) list(ctx context.Context, userID int64) (managementResult, *editRequest, error) {

	return f.loadAndSave(ctx, userID, list, session{})
}

func (f *EventManagementService) loadAndSave(ctx context.Context, userID int64, state fsm.StateID, s session) (managementResult, *editRequest, error) {
	result, err := f.loadPage(ctx, state, s)
	if err != nil {
		return managementResult{}, nil, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil, nil
}

func validField(field domain.EventField) bool {
	switch field {
	case domain.EventTitle, domain.EventDescription, domain.EventPhoto, domain.EventLocation, domain.EventDates, domain.EventCapacity, domain.EventTags:
		return true
	}
	return false
}

func (f *EventManagementService) loadPage(ctx context.Context, state fsm.StateID, s session) (managementResult, error) {
	result := managementResult{state: state, session: s}
	if state == list {
		events, err := f.repo.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return managementResult{}, err
		}
		result.more = len(events) > pageSize
		if result.more {
			events = events[:pageSize]
		}
		result.events = events
		return result, nil
	}

	event, err := f.repo.Get(ctx, s.eventID)
	if err != nil {
		return managementResult{}, err
	}
	result.event = event
	switch state {
	case card:
		result.location, err = time.LoadLocation("Asia/Yekaterinburg")
		if err != nil {
			return managementResult{}, err
		}
	case participants:
		users, err := f.users.ListForEvent(ctx, s.eventID, pageSize+1, s.page*pageSize)
		if err != nil {
			return managementResult{}, err
		}
		result.more = len(users) > pageSize
		if result.more {
			users = users[:pageSize]
		}
		result.users = users
	}
	return result, nil
}

func (f *EventManagementService) restore(ctx context.Context, userID int64, state fsm.StateID, s session) (managementResult, error) {
	result, _, err := f.loadAndSave(ctx, userID, state, s)
	return result, err
}
