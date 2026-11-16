package manage

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/pagination"
)

type Repository interface {
	List(context.Context, int, int) ([]domain.EventSummary, error)
	Get(context.Context, int64) (domain.Event, error)
	Delete(context.Context, int64) error
}

type Editor interface {
	Edit(context.Context, *bot.Bot, int64, int64, domain.Event, domain.EventField) error
}

type UserRepository interface {
	ListForEvent(context.Context, int64, int, int) ([]domain.User, error)
}

type mode int

const (
	list mode = iota
	card
	fields
	confirmDelete
	participants
)
const pageSize = pagination.Size

type session struct {
	token   string
	mode    mode
	page    int
	eventID int64
}
type editRequest struct {
	event domain.Event
	field domain.EventField
}

type Flow struct {
	mu       sync.Mutex
	sessions map[int64]session
	repo     Repository
	editor   Editor
	users    UserRepository
	adminIDs []int64
}

func New(repo Repository, editor Editor, users UserRepository, adminIDs []int64) *Flow {
	return &Flow{sessions: make(map[int64]session), repo: repo, editor: editor, users: users, adminIDs: slices.Clone(adminIDs)}
}
func (f *Flow) Cancel(id int64) { f.mu.Lock(); defer f.mu.Unlock(); delete(f.sessions, id) }

func (f *Flow) process(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists := f.sessions[userID]
	if callback == "admin:events" {
		s = session{mode: list}
	} else {
		parts := strings.Split(callback, ":")
		if !exists || len(parts) != 4 || parts[2] != s.token {
			return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
		}
		action := parts[3]
		switch {
		case strings.HasPrefix(action, "page") && (s.mode == list || s.mode == participants):
			page, ok := pagination.Move(s.page, action)
			if ok {
				s.page = page
			}
		case strings.HasPrefix(action, "open"):
			id, err := strconv.ParseInt(strings.TrimPrefix(action, "open"), 10, 32)
			if err != nil || id <= 0 {
				return navigation.Page{}, nil, errors.New("invalid event ID")
			}
			s.eventID, s.mode = id, card
		case action == "edit" && s.mode == card:
			s.mode = fields
		case action == "participants" && s.mode == card:
			s.mode, s.page = participants, 0
		case strings.HasPrefix(action, "field") && s.mode == fields:
			field := domain.EventField(strings.TrimPrefix(action, "field"))
			if !validField(field) {
				return navigation.Page{}, nil, errors.New("invalid event field")
			}
			event, err := f.repo.Get(ctx, s.eventID)
			if err != nil {
				return navigation.Page{}, nil, err
			}
			delete(f.sessions, userID)
			return navigation.Page{}, &editRequest{event, field}, nil
		case action == "delete" && s.mode == card:
			s.mode = confirmDelete
		case action == "keep" && s.mode == confirmDelete:
			s.mode = card
		case action == "confirm" && s.mode == confirmDelete:
			if err := f.repo.Delete(ctx, s.eventID); err != nil {
				return navigation.Page{}, nil, err
			}
			delete(f.sessions, userID)
			return navigation.Page{Key: "admin:events:deleted", Text: "Мероприятие удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}, nil, nil
		}
	}
	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	f.sessions[userID] = s
	return page, nil, nil
}

func validField(field domain.EventField) bool {
	switch field {
	case domain.EventTitle, domain.EventDescription, domain.EventPhoto, domain.EventLocation, domain.EventDates, domain.EventCapacity, domain.EventTags:
		return true
	}
	return false
}
