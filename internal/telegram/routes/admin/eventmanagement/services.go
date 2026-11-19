package eventmanagement

import (
	"context"
	"errors"
	"slices"
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

type managementResult struct {
	state    fsm.StateID
	session  session
	event    domain.Event
	events   []domain.EventSummary
	users    []domain.User
	more     bool
	location *time.Location
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

func (f *Service) begin(ctx context.Context, userID int64) (managementResult, error) {
	return f.loadAndSave(ctx, userID, list, session{})
}

func (f *Service) changePage(ctx context.Context, userID int64, state fsm.StateID, s session, page int) (managementResult, error) {
	s.page = page
	return f.loadAndSave(ctx, userID, state, s)
}

func (f *Service) openEvent(ctx context.Context, userID int64, s session, eventID int64) (managementResult, error) {
	s.eventID = eventID
	return f.loadAndSave(ctx, userID, card, s)
}

func (f *Service) showFields(ctx context.Context, userID int64, s session) (managementResult, error) {
	return f.loadAndSave(ctx, userID, fields, s)
}

func (f *Service) showParticipants(ctx context.Context, userID int64, s session) (managementResult, error) {
	s.page = 0
	return f.loadAndSave(ctx, userID, participants, s)
}

func (f *Service) eventForEdit(ctx context.Context, eventID int64, field domain.EventField) (domain.Event, error) {
	if !validField(field) {
		return domain.Event{}, errors.New("invalid event field")
	}
	return f.repo.Get(ctx, eventID)
}

func (f *Service) showDeleteConfirmation(ctx context.Context, userID int64, s session) (managementResult, error) {
	return f.loadAndSave(ctx, userID, confirmDelete, s)
}

func (f *Service) deleteEvent(ctx context.Context, userID, eventID int64) error {
	if err := f.repo.Delete(ctx, eventID); err != nil {
		return err
	}
	f.machine.Clear(userID)
	return nil
}

func (f *Service) loadAndSave(ctx context.Context, userID int64, state fsm.StateID, s session) (managementResult, error) {
	s.RotateScreenToken()
	result, err := f.loadPage(ctx, state, s)
	if err != nil {
		return managementResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
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
