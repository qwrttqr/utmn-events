package tags

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
)

type Repository interface {
	Create(context.Context, string) (domain.Tag, error)
	List(context.Context, int, int) ([]domain.Tag, error)
	ForEvent(context.Context, int64) ([]domain.Tag, error)
	SetForEvent(context.Context, int64, []int64) error
}

type EventRepository interface {
	List(context.Context, int, int) ([]domain.EventSummary, error)
}

const (
	browse      fsm.StateID = "admin.tags.browse"
	createName  fsm.StateID = "admin.tags.create"
	chooseEvent fsm.StateID = "admin.tags.choose_event"
	editTags    fsm.StateID = "admin.tags.edit"
)

const pageSize = 8

type session struct {
	page     int
	event    domain.EventSummary
	selected map[int64]bool
	tags     []domain.Tag
	events   []domain.EventSummary
	keyboards.TokenizedDraft
}

var (
	errTagSessionMissing = errors.New("tag session missing")
	errTagActionStale    = errors.New("tag action stale")
	errTagNameInvalid    = errors.New("invalid tag name")
)

type tagResult struct {
	state   fsm.StateID
	session session
	more    bool
	name    string
}

type Service struct {
	machine *fsm.FSM
	repo    Repository
	events  EventRepository
}

func NewService(machine *fsm.FSM, repo Repository, events EventRepository) *Service {
	return &Service{machine: machine, repo: repo, events: events}
}

func (f *Service) begin(ctx context.Context, id int64) (tagResult, error) {
	return f.loadAndSave(ctx, id, browse, session{selected: make(map[int64]bool)})
}

func (f *Service) callbackSession(id int64, token string) (fsm.StateID, session, error) {
	state, s, exists := fsm.GetDraft[session](f.machine, id)
	if !exists {
		return fsm.Idle, session{}, errTagSessionMissing
	}
	if !s.IsTokenValid(token) {
		return fsm.Idle, session{}, errTagActionStale
	}
	return state, s, nil
}

func (f *Service) create(ctx context.Context, id int64, token string) (tagResult, error) {
	_, s, err := f.callbackSession(id, token)
	if err != nil {
		return tagResult{}, err
	}
	return f.loadAndSave(ctx, id, createName, s)
}

func (f *Service) assign(ctx context.Context, id int64, token string) (tagResult, error) {
	_, s, err := f.callbackSession(id, token)
	if err != nil {
		return tagResult{}, err
	}
	s.page = 0
	return f.loadAndSave(ctx, id, chooseEvent, s)
}

func (f *Service) changePage(ctx context.Context, id int64, token string, page int64) (tagResult, error) {
	state, s, err := f.callbackSession(id, token)
	if err != nil {
		return tagResult{}, err
	}
	if page >= 0 && (page == int64(s.page-1) || page == int64(s.page+1)) {
		s.page = int(page)
	}
	return f.loadAndSave(ctx, id, state, s)
}

func (f *Service) selectEvent(ctx context.Context, id int64, token string, eventID int64) (tagResult, error) {
	_, s, err := f.callbackSession(id, token)
	if err != nil {
		return tagResult{}, err
	}
	index := slices.IndexFunc(s.events, func(event domain.EventSummary) bool { return event.ID == eventID })
	if index < 0 {
		return tagResult{}, errTagActionStale
	}
	assigned, err := f.repo.ForEvent(ctx, eventID)
	if err != nil {
		return tagResult{}, err
	}
	s.selected = make(map[int64]bool, len(assigned))
	for _, tag := range assigned {
		s.selected[tag.ID] = true
	}
	s.event, s.page = s.events[index], 0
	return f.loadAndSave(ctx, id, editTags, s)
}

func (f *Service) toggleTag(ctx context.Context, id int64, token string, tagID int64) (tagResult, error) {
	_, s, err := f.callbackSession(id, token)
	if err != nil {
		return tagResult{}, err
	}
	if !slices.ContainsFunc(s.tags, func(tag domain.Tag) bool { return tag.ID == tagID }) {
		return tagResult{}, errTagActionStale
	}
	s.selected = maps.Clone(s.selected)
	if s.selected[tagID] {
		delete(s.selected, tagID)
	} else {
		s.selected[tagID] = true
	}
	return f.loadAndSave(ctx, id, editTags, s)
}

func (f *Service) clearTags(ctx context.Context, id int64, token string) (tagResult, error) {
	_, s, err := f.callbackSession(id, token)
	if err != nil {
		return tagResult{}, err
	}
	s.selected = make(map[int64]bool)
	return f.loadAndSave(ctx, id, editTags, s)
}

func (f *Service) saveTags(ctx context.Context, id int64, token string) (tagResult, error) {
	_, s, err := f.callbackSession(id, token)
	if err != nil {
		return tagResult{}, err
	}
	var ids []int64
	for tagID := range s.selected {
		ids = append(ids, tagID)
	}
	if err := f.repo.SetForEvent(ctx, s.event.ID, ids); err != nil {
		return tagResult{}, err
	}
	f.machine.Clear(id)
	return tagResult{state: fsm.Idle}, nil
}

func (f *Service) name(ctx context.Context, id int64, text string) (tagResult, error) {
	_, s, exists := fsm.GetDraft[session](f.machine, id)
	if !exists {
		return tagResult{}, errTagSessionMissing
	}
	name := strings.ToLower(strings.TrimSpace(text))
	if name == "" || utf8.RuneCountInString(name) > 40 || strings.ContainsAny(name, ";/") || strings.ContainsFunc(name, unicode.IsControl) {
		return tagResult{}, errTagNameInvalid
	}
	tag, err := f.repo.Create(ctx, name)
	if err != nil {
		return tagResult{}, err
	}
	s.RotateScreenToken()
	f.machine.Replace(id, browse, s)
	return tagResult{state: browse, name: tag.Name, session: s}, nil
}

func (f *Service) loadPage(ctx context.Context, state fsm.StateID, s *session) (tagResult, error) {
	result := tagResult{state: state}
	switch state {
	case createName:
	case chooseEvent:
		events, err := f.events.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return tagResult{}, err
		}
		result.more = len(events) > pageSize
		if result.more {
			events = events[:pageSize]
		}
		s.events = events
	default:
		tags, err := f.repo.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return tagResult{}, err
		}
		result.more = len(tags) > pageSize
		if result.more {
			tags = tags[:pageSize]
		}
		s.tags = tags
	}
	result.session = *s
	return result, nil
}

func cloneSession(s session) session {
	s.selected = maps.Clone(s.selected)
	s.tags = slices.Clone(s.tags)
	s.events = slices.Clone(s.events)
	return s
}

func (f *Service) loadAndSave(ctx context.Context, userID int64, state fsm.StateID, s session) (tagResult, error) {
	s.RotateScreenToken()
	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return tagResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *Service) snapshot(userID int64) (fsm.StateID, *session) {
	state, s, exists := fsm.GetDraft[session](f.machine, userID)
	if !exists {
		return fsm.Idle, nil
	}
	saved := cloneSession(s)
	return state, &saved
}
