package events

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
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
	keyboards.TokenizedDraft
}

type eventsResult struct {
	state    fsm.StateID
	session  session
	message  string
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

func (f *EventsService) browse(ctx context.Context, userID int64) (eventsResult, error) {
	return f.loadAndSave(ctx, userID, all, session{})
}

func (f *EventsService) myRegistrations(ctx context.Context, userID int64) (eventsResult, error) {
	return f.loadAndSave(ctx, userID, mine, session{})
}

func (f *EventsService) chooseTags(ctx context.Context, userID int64) (eventsResult, error) {
	return f.loadAndSave(ctx, userID, chooseTags, session{selected: make(map[int64]string)})
}

func (f *EventsService) changePage(ctx context.Context, userID int64, state fsm.StateID, s session, page int) (eventsResult, error) {
	if page >= 0 && (page == s.page-1 || page == s.page+1) {
		s.page = page
	}
	return f.loadAndSave(ctx, userID, state, s)
}

func (f *EventsService) open(ctx context.Context, userID int64, s session, eventID int64) (eventsResult, error) {
	s.eventID = eventID
	return f.loadAndSave(ctx, userID, card, s)
}

func (f *EventsService) toggleTag(ctx context.Context, userID int64, s session, tagID int64) (eventsResult, error) {
	for _, tag := range s.tags {
		if tag.ID != tagID {
			continue
		}
		s.selected = maps.Clone(s.selected)
		if _, selected := s.selected[tagID]; selected {
			delete(s.selected, tagID)
		} else {
			s.selected[tagID] = tag.Name
		}
		break
	}
	return f.loadAndSave(ctx, userID, chooseTags, s)
}

func (f *EventsService) clearTags(ctx context.Context, userID int64, s session) (eventsResult, error) {
	s.selected = make(map[int64]string)
	return f.loadAndSave(ctx, userID, chooseTags, s)
}

func (f *EventsService) search(ctx context.Context, userID int64, s session) (eventsResult, error) {
	if len(s.selected) == 0 {
		result, err := f.loadAndSave(ctx, userID, chooseTags, s)
		result.message = "Выберите хотя бы один тег."
		return result, err
	}
	s.page = 0
	return f.loadAndSave(ctx, userID, results, s)
}

func (f *EventsService) enroll(ctx context.Context, userID int64, s session) (eventsResult, error) {
	var message string
	err := f.registrations.Enroll(ctx, s.eventID, userID)
	switch {
	case err == nil:
		message = "Вы записаны на мероприятие."
	case errors.Is(err, domain.ErrEventFull):
		message = "Свободных мест больше нет."
	case errors.Is(err, domain.ErrEventClosed):
		message = "Запись на это мероприятие уже закрыта."
	default:
		return eventsResult{}, err
	}
	result, err := f.loadAndSave(ctx, userID, card, s)
	result.message = message
	return result, err
}

func (f *EventsService) cancelEnrollment(ctx context.Context, userID int64, s session) (eventsResult, error) {
	return f.loadAndSave(ctx, userID, cancelEnrollment, s)
}

func (f *EventsService) keepEnrollment(ctx context.Context, userID int64, s session) (eventsResult, error) {
	return f.loadAndSave(ctx, userID, card, s)
}

func (f *EventsService) confirmCancellation(ctx context.Context, userID int64, s session) (eventsResult, error) {
	if err := f.registrations.Unenroll(ctx, s.eventID, userID); err != nil {
		return eventsResult{}, err
	}
	f.machine.Clear(userID)
	return eventsResult{state: fsm.Idle}, nil
}

func (f *EventsService) loadAndSave(ctx context.Context, userID int64, state fsm.StateID, s session) (eventsResult, error) {
	s.RotateScreenToken()
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
	return f.loadAndSave(ctx, userID, state, cloneSession(saved))
}
