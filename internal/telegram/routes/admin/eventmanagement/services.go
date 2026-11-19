package eventmanagement

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
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
	keyboards.TokenizedDraft
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

type Service struct {
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
) *Service {
	return &Service{
		machine:  machine,
		repo:     repo,
		users:    users,
		adminIDs: slices.Clone(adminIDs),
	}
}
func (f *Service) Cancel(id int64) {
	switch f.machine.CurrentState(id) {
	case list, card, fields, confirmDelete, participants:
		f.machine.Clear(id)
	}
}

func (f *Service) process(ctx context.Context, userID int64, callback string) (managementResult, *editRequest, error) {
	if callback == "admin:events" {
		return f.loadAndSave(ctx, userID, list, session{})
	}

	expired := managementResult{status: managementExpired}
	callback, token, ok := keyboards.ParseScreenCallback(callback)
	state, s, exists := fsm.GetDraft[session](f.machine, userID)
	if !ok || !exists || !s.IsTokenValid(token) {
		return expired, nil, nil
	}
	data, ok := strings.CutPrefix(callback, callbackPrefix)
	if !ok {
		return expired, nil, nil
	}
	id, action, ok := strings.Cut(data, ":")
	eventID, err := strconv.ParseInt(id, 10, 64)
	if !ok || err != nil || eventID < 0 || strings.Contains(action, ":") {
		return expired, nil, nil
	}
	selectedEvent := eventID > 0 && eventID == s.eventID

	switch {
	case strings.HasPrefix(action, "page") && ((state == list && eventID == 0) || (state == participants && selectedEvent)):
		if nextPage, ok := pagination.Move(s.page, action); ok {
			s.page = nextPage
		}
	case action == "open" && state == list && eventID > 0:
		s.eventID, state = eventID, card
	case action == "edit" && state == card && selectedEvent:
		state = fields
	case action == "participants" && state == card && selectedEvent:
		state, s.page = participants, 0
	case strings.HasPrefix(action, "field") && state == fields && selectedEvent:
		field := domain.EventField(strings.TrimPrefix(action, "field"))
		if !validField(field) {
			return managementResult{}, nil, errors.New("invalid event field")
		}
		event, err := f.repo.Get(ctx, eventID)
		if err != nil {
			return managementResult{}, nil, err
		}
		return managementResult{}, &editRequest{event, field}, nil
	case action == "delete" && state == card && selectedEvent:
		state = confirmDelete
	case action == "keep" && state == confirmDelete && selectedEvent:
		state = card
	case action == "confirm" && state == confirmDelete && selectedEvent:
		if err := f.repo.Delete(ctx, eventID); err != nil {
			return managementResult{}, nil, err
		}
		f.machine.Clear(userID)
		return managementResult{status: managementDeleted}, nil, nil
	default:
		return expired, nil, nil
	}
	return f.loadAndSave(ctx, userID, state, s)
}

func (f *Service) loadAndSave(ctx context.Context, userID int64, state fsm.StateID, s session) (managementResult, *editRequest, error) {
	s.RotateScreenToken()
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

func (f *Service) loadPage(ctx context.Context, state fsm.StateID, s session) (managementResult, error) {
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

func (f *Service) restore(ctx context.Context, userID int64, state fsm.StateID, s session) (managementResult, error) {
	result, _, err := f.loadAndSave(ctx, userID, state, s)
	return result, err
}
