package tags

import (
	"context"
	"crypto/rand"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
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
	token    string
	page     int
	event    domain.EventSummary
	selected map[int64]bool
	tags     []domain.Tag
	events   []domain.EventSummary
}

type tagStatus uint8

const (
	tagsLoadFailed tagStatus = iota + 1
	tagSessionMissing
	tagActionStale
	tagAssignmentsSaved
	tagNameInvalid
	tagCreated
	tagActionFailed
)

type tagResult struct {
	state       fsm.StateID
	session     session
	more        bool
	status      tagStatus
	name        string
	retryToken  string
	retryAction string
}

type TagsService struct {
	machine *fsm.FSM
	repo    Repository
	events  EventRepository
}

func NewService(machine *fsm.FSM, repo Repository, events EventRepository) *TagsService {
	return &TagsService{machine: machine, repo: repo, events: events}
}

func (f *TagsService) enteringName(id int64) bool {
	state, _, exists := fsm.Load[session](f.machine, id)
	return exists && state == createName
}

func (f *TagsService) begin(ctx context.Context, id int64) (tagResult, error) {
	state, s := browse, session{token: rand.Text(), selected: make(map[int64]bool)}
	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return tagResult{status: tagsLoadFailed}, err
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) create(ctx context.Context, id int64, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}
	if action == "create" {
		state = createName
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) assign(ctx context.Context, id int64, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}
	if action == "assign" {
		state, s.page = chooseEvent, 0
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) changePage(ctx context.Context, id int64, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}
	if strings.HasPrefix(action, "page") {
		page, err := strconv.Atoi(strings.TrimPrefix(action, "page"))
		if err == nil && page >= 0 && (page == s.page-1 || page == s.page+1) {
			s.page = page
		}
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) selectEvent(ctx context.Context, id int64, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}
	if strings.HasPrefix(action, "event") && state == chooseEvent {
		eventID, _ := strconv.ParseInt(strings.TrimPrefix(action, "event"), 10, 64)
		for _, event := range s.events {
			if event.ID != eventID {
				continue
			}
			assigned, err := f.repo.ForEvent(ctx, eventID)
			if err != nil {
				return failure(err)
			}
			s.selected = make(map[int64]bool)
			for _, tag := range assigned {
				s.selected[tag.ID] = true
			}
			s.event, state, s.page = event, editTags, 0
			break
		}
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) toggleTag(ctx context.Context, id int64, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}
	if strings.HasPrefix(action, "toggle") && state == editTags {
		tagID, _ := strconv.ParseInt(strings.TrimPrefix(action, "toggle"), 10, 64)
		for _, tag := range s.tags {
			if tag.ID == tagID {
				if s.selected[tagID] {
					delete(s.selected, tagID)
				} else {
					s.selected[tagID] = true
				}
				break
			}
		}
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) clearTags(ctx context.Context, id int64, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}
	if action == "clear" && state == editTags {
		s.selected = make(map[int64]bool)
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) saveTags(ctx context.Context, id int64, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}
	if action == "save" && state == editTags {
		var ids []int64
		for id := range s.selected {
			ids = append(ids, id)
		}
		if err := f.repo.SetForEvent(ctx, s.event.ID, ids); err != nil {
			return failure(err)
		}
		f.machine.Clear(id)
		return tagResult{status: tagAssignmentsSaved}, nil
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) staleCallback(ctx context.Context, id int64, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) name(ctx context.Context, id int64, text, callback string) (tagResult, error) {
	state, s, exists := fsm.Load[session](f.machine, id)
	if !exists {
		return tagResult{status: tagSessionMissing}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return tagResult{status: tagActionStale}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (tagResult, error) {
		return tagResult{status: tagActionFailed, retryToken: old.token, retryAction: action}, err
	}
	if callback == "" && state == createName {
		name := strings.ToLower(strings.TrimSpace(text))
		if name == "" || utf8.RuneCountInString(name) > 40 || strings.ContainsAny(name, ";/") || strings.ContainsFunc(name, unicode.IsControl) {
			return tagResult{status: tagNameInvalid}, nil
		}
		tag, err := f.repo.Create(ctx, name)
		if err != nil {
			return failure(err)
		}
		state = browse
		f.machine.Replace(id, state, s)
		return tagResult{status: tagCreated, name: tag.Name, session: s}, nil
	}

	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return failure(err)
	}
	f.machine.Replace(id, state, s)
	return result, nil
}

func (f *TagsService) loadPage(ctx context.Context, state fsm.StateID, s *session) (tagResult, error) {
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

func (f *TagsService) restore(ctx context.Context, userID int64, state fsm.StateID, saved session) (tagResult, error) {
	s := cloneSession(saved)
	s.token = rand.Text()
	result, err := f.loadPage(ctx, state, &s)
	if err != nil {
		return tagResult{}, err
	}
	f.machine.Replace(userID, state, s)
	return result, nil
}

func (f *TagsService) snapshot(userID int64) (fsm.StateID, *session) {
	state, s, exists := fsm.Load[session](f.machine, userID)
	if !exists {
		return fsm.Idle, nil
	}
	saved := cloneSession(s)
	return state, &saved
}
