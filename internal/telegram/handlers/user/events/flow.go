package events

import (
	"context"
	"crypto/rand"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
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

type mode int

const (
	all mode = iota
	mine
	chooseTags
	results
	card
	cancelEnrollment
)

type session struct {
	token    string
	mode     mode
	page     int
	eventID  int64
	selected map[int64]string
	tags     []domain.Tag
}

type Flow struct {
	mu            sync.Mutex
	sessions      map[int64]session
	repo          Repository
	tags          TagRepository
	registrations RegistrationRepository
	location      *time.Location
	now           func() time.Time
}

func New(repo Repository, tags TagRepository, registrations RegistrationRepository) (*Flow, error) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return nil, err
	}
	return &Flow{sessions: make(map[int64]session), repo: repo, tags: tags, registrations: registrations, location: location, now: time.Now}, nil
}

func (f *Flow) Cancel(id int64) { f.mu.Lock(); defer f.mu.Unlock(); delete(f.sessions, id) }

func cloneSession(s session) session {
	s.selected = maps.Clone(s.selected)
	s.tags = slices.Clone(s.tags)
	return s
}

func (f *Flow) process(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists := f.sessions[userID]
	s = cloneSession(s)
	notice := ""
	switch callback {
	case "user:events":
		s = session{mode: all, selected: make(map[int64]string)}
	case "user:registrations":
		s = session{mode: mine, selected: make(map[int64]string)}
	case "user:search":
		s = session{mode: chooseTags, selected: make(map[int64]string)}
	default:
		parts := strings.Split(callback, ":")
		if !exists || len(parts) != 4 || parts[2] != s.token {
			return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
		}
		action := parts[3]
		switch {
		case strings.HasPrefix(action, "page") && (s.mode == all || s.mode == mine || s.mode == results || s.mode == chooseTags):
			if page, ok := pagination.Move(s.page, action); ok {
				s.page = page
			}
		case strings.HasPrefix(action, "open"):
			id, err := strconv.ParseInt(strings.TrimPrefix(action, "open"), 10, 32)
			if err != nil || id <= 0 {
				return navigation.Page{}, errors.New("invalid event ID")
			}
			s.eventID, s.mode = id, card
		case strings.HasPrefix(action, "toggle") && s.mode == chooseTags:
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
		case action == "clear" && s.mode == chooseTags:
			s.selected = make(map[int64]string)
		case action == "search" && s.mode == chooseTags:
			if len(s.selected) == 0 {
				notice = "Выберите хотя бы один тег."
			} else {
				s.mode, s.page = results, 0
			}
		case action == "enroll" && s.mode == card:
			err := f.registrations.Enroll(ctx, s.eventID, userID)
			switch {
			case err == nil:
				notice = "Вы записаны на мероприятие."
			case errors.Is(err, domain.ErrEventFull):
				notice = "Свободных мест больше нет."
			case errors.Is(err, domain.ErrEventClosed):
				notice = "Запись на это мероприятие уже закрыта."
			default:
				return navigation.Page{}, err
			}
		case action == "cancel" && s.mode == card:
			s.mode = cancelEnrollment
		case action == "keep" && s.mode == cancelEnrollment:
			s.mode = card
		case action == "confirm" && s.mode == cancelEnrollment:
			if err := f.registrations.Unenroll(ctx, s.eventID, userID); err != nil {
				return navigation.Page{}, err
			}
			s.mode, notice = card, "Запись на мероприятие отменена."
		}
	}
	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	f.sessions[userID] = s
	return page, nil
}
