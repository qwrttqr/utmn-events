package events

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/pagination"
)

type Repository interface {
	Browse(context.Context, domain.EventFilter, int, int) ([]domain.EventSummary, error)
	Get(context.Context, int64) (domain.Event, error)
}
type TagRepository interface {
	List(context.Context, int, int) ([]domain.Tag, error)
}
type RegistrationRepository interface {
	IsEnrolled(context.Context, int64, int64) (bool, error)
	Enroll(context.Context, int64, int64) error
	Unenroll(context.Context, int64, int64) error
}

const (
	all              fsm.StateID = "user.events.all"
	mine             fsm.StateID = "user.events.mine"
	chooseTags       fsm.StateID = "user.events.choose_tags"
	results          fsm.StateID = "user.events.results"
	card             fsm.StateID = "user.events.card"
	cancelEnrollment fsm.StateID = "user.events.cancel"
)

type session struct {
	page     int
	eventID  int64
	selected map[int64]string
	tags     []domain.Tag
}

type eventsStatus uint8

const (
	eventsExpired eventsStatus = iota + 1
	enrollmentCancelled
	tagsRequired
	enrollmentSaved
	enrollmentFull
	enrollmentClosed
)

type eventsResult struct {
	state    fsm.StateID
	session  session
	status   eventsStatus
	event    domain.Event
	events   []domain.EventSummary
	enrolled bool
	more     bool
}

type EventsService struct {
	machine       *fsm.FSM
	repo          Repository
	tags          TagRepository
	registrations RegistrationRepository
	location      *time.Location
	now           func() time.Time
}

func NewService(machine *fsm.FSM, repo Repository, tags TagRepository, registrations RegistrationRepository) (*EventsService, error) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return nil, err
	}
	return &EventsService{machine: machine, repo: repo, tags: tags, registrations: registrations, location: location, now: time.Now}, nil
}

func (f *EventsService) Cancel(id int64) {
	switch f.machine.CurrentState(id) {
	case all, mine, chooseTags, results, card, cancelEnrollment:
		f.machine.Clear(id)
	}
}

func cloneSession(s session) session {
	s.selected = maps.Clone(s.selected)
	s.tags = slices.Clone(s.tags)
	return s
}

func (f *EventsService) process(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	if callback == "user:events" {
		return f.browse(ctx, userID)
	}
	if callback == "user:registrations" {
		return f.myRegistrations(ctx, userID)
	}
	if callback == "user:search" {
		return f.chooseTags(ctx, userID)
	}
	_, action, valid := parseCallback(callback)
	if !valid {
		return eventsResult{status: eventsExpired}, nil
	}
	if strings.HasPrefix(action, "page") {
		return f.page(ctx, userID, callback)
	}
	if action == "open" {
		return f.open(ctx, userID, callback)
	}
	if strings.HasPrefix(action, "toggle") {
		return f.toggleTag(ctx, userID, callback)
	}
	if action == "clear" {
		return f.clearTags(ctx, userID, callback)
	}
	if action == "search" {
		return f.search(ctx, userID, callback)
	}
	if action == "enroll" {
		return f.enroll(ctx, userID, callback)
	}
	if action == "cancel" {
		return f.cancelEnrollment(ctx, userID, callback)
	}
	if action == "keep" {
		return f.keepEnrollment(ctx, userID, callback)
	}
	if action == "confirm" {
		return f.confirmCancellation(ctx, userID, callback)
	}
	return eventsResult{status: eventsExpired}, nil
}

func (f *EventsService) page(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	s = cloneSession(s)

	eventID, action, valid := parseCallback(callback)
	pageNumber, matchesView := strings.CutPrefix(action, pageActionPrefix(state))
	if !exists || !valid || eventID != 0 || !matchesView || (state != all && state != mine && state != results && state != chooseTags) {
		return eventsResult{status: eventsExpired}, nil
	}
	s.page, _ = pagination.Move(s.page, "page"+pageNumber)

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}

	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) open(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	s = cloneSession(s)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || eventID == 0 || action != "open" || (state != all && state != mine && state != results) {
		return eventsResult{status: eventsExpired}, nil
	}

	s.eventID, state = eventID, card

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}

	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) toggleTag(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	s = cloneSession(s)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || eventID != 0 || !strings.HasPrefix(action, "toggle") || state != chooseTags {
		return eventsResult{status: eventsExpired}, nil
	}

	id, _ := strconv.ParseInt(strings.TrimPrefix(action, "toggle"), 10, 64)
	for _, tag := range s.tags {
		if tag.ID != id {
			continue
		}
		if _, selected := s.selected[id]; selected {
			delete(s.selected, id)
		} else {
			s.selected[id] = tag.Name
		}
		break
	}

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) clearTags(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	s = cloneSession(s)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || eventID != 0 || action != "clear" || state != chooseTags {
		return eventsResult{status: eventsExpired}, nil
	}

	s.selected = make(map[int64]string)

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) search(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	s = cloneSession(s)
	var notice eventsStatus

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || eventID != 0 || action != "search" || state != chooseTags {
		return eventsResult{status: eventsExpired}, nil
	}

	if len(s.selected) == 0 {
		notice = tagsRequired
	} else {
		state, s.page = results, 0
	}

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	result.status = notice
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) enroll(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	s = cloneSession(s)
	var notice eventsStatus

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || eventID == 0 || eventID != s.eventID || action != "enroll" || state != card {
		return eventsResult{status: eventsExpired}, nil
	}

	err := f.registrations.Enroll(ctx, eventID, userID)
	switch {
	case err == nil:
		notice = enrollmentSaved
	case errors.Is(err, domain.ErrEventFull):
		notice = enrollmentFull
	case errors.Is(err, domain.ErrEventClosed):
		notice = enrollmentClosed
	default:
		return eventsResult{}, err
	}

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	result.status = notice
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) cancelEnrollment(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	s = cloneSession(s)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || eventID == 0 || eventID != s.eventID || action != "cancel" || state != card {
		return eventsResult{status: eventsExpired}, nil
	}

	state = cancelEnrollment

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) keepEnrollment(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	s = cloneSession(s)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || eventID == 0 || eventID != s.eventID || action != "keep" || state != cancelEnrollment {
		return eventsResult{status: eventsExpired}, nil
	}

	state = card

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) confirmCancellation(ctx context.Context, userID int64, callback string) (eventsResult, error) {
	state, s, exists := fsm.Load[session](f.machine, userID)

	eventID, action, valid := parseCallback(callback)
	if !exists || !valid || eventID == 0 || eventID != s.eventID || action != "confirm" || state != cancelEnrollment {
		return eventsResult{status: eventsExpired}, nil
	}

	if err := f.registrations.Unenroll(ctx, eventID, userID); err != nil {
		return eventsResult{}, err
	}
	f.machine.Clear(userID)
	return eventsResult{status: enrollmentCancelled}, nil
}

func (f *EventsService) browse(ctx context.Context, userID int64) (eventsResult, error) {

	state, s := all, session{selected: make(map[int64]string)}

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) myRegistrations(ctx context.Context, userID int64) (eventsResult, error) {

	state, s := mine, session{selected: make(map[int64]string)}

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) chooseTags(ctx context.Context, userID int64) (eventsResult, error) {

	state, s := chooseTags, session{selected: make(map[int64]string)}

	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *EventsService) loadPage(ctx context.Context, userID int64, state fsm.StateID, s *session) (eventsResult, error) {
	result := eventsResult{state: state}
	switch state {
	case chooseTags:
		tags, err := f.tags.List(ctx, pagination.Size+1, s.page*pagination.Size)
		if err != nil {
			return eventsResult{}, err
		}
		result.more = len(tags) > pagination.Size
		if result.more {
			tags = tags[:pagination.Size]
		}
		s.tags = tags
	case all, mine, results:
		filter := domain.EventFilter{UpcomingOnly: state != mine}
		if state == mine {
			filter.UserTelegramID = userID
		}
		if state == results {
			if len(s.selected) == 0 {
				return eventsResult{}, fmt.Errorf("tag search requires at least one tag")
			}
			for id := range s.selected {
				filter.TagIDs = append(filter.TagIDs, id)
			}
			slices.Sort(filter.TagIDs)
		}
		events, err := f.repo.Browse(ctx, filter, pagination.Size+1, s.page*pagination.Size)
		if err != nil {
			return eventsResult{}, err
		}
		result.more = len(events) > pagination.Size
		if result.more {
			events = events[:pagination.Size]
		}
		result.events = events
	case card, cancelEnrollment:
		event, err := f.repo.Get(ctx, s.eventID)
		if err != nil {
			return eventsResult{}, err
		}
		enrolled, err := f.registrations.IsEnrolled(ctx, s.eventID, userID)
		if err != nil {
			return eventsResult{}, err
		}
		result.event, result.enrolled = event, enrolled
	}
	result.session = *s
	return result, nil
}

func (f *EventsService) restore(ctx context.Context, userID int64, state fsm.StateID, saved session) (eventsResult, error) {
	s := cloneSession(saved)
	result, err := f.loadPage(ctx, userID, state, &s)
	if err != nil {
		return eventsResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}
