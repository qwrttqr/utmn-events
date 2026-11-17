package event_management

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/navigation"
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

type EventManagementService struct {
	mu       sync.Mutex
	machine  *fsm.FSM[string, session]
	repo     Repository
	users    UserRepository
	adminIDs []int64
}

func NewService(
	repo Repository,
	users UserRepository,
	adminIDs []int64,
) *EventManagementService {
	return &EventManagementService{
		machine:  fsm.NewInMemoryFSM[string, session]("idle"),
		repo:     repo,
		users:    users,
		adminIDs: slices.Clone(adminIDs),
	}
}
func (f *EventManagementService) Cancel(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clear(id)
}

func (f *EventManagementService) process(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	if callback == "admin:events" {
		return f.list(ctx, userID, callback)
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 {
		return f.staleCallback(ctx, userID, callback)
	}
	action := parts[3]
	if strings.HasPrefix(action, "page") {
		return f.page(ctx, userID, callback)
	}
	if strings.HasPrefix(action, "open") {
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
	return f.staleCallback(ctx, userID, callback)
}

func (f *EventManagementService) page(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}
	action := parts[3]

	if strings.HasPrefix(action, "page") && (s.mode == list || s.mode == participants) {
		page, ok := pagination.Move(s.page, action)
		if ok {
			s.page = page
		}
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) open(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}
	action := parts[3]

	if strings.HasPrefix(action, "open") {
		id, err := strconv.ParseInt(strings.TrimPrefix(action, "open"), 10, 32)
		if err != nil || id <= 0 {
			return navigation.Page{}, nil, errors.New("invalid event ID")
		}
		s.eventID, s.mode = id, card
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) edit(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}
	action := parts[3]

	if action == "edit" && s.mode == card {
		s.mode = fields
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) participants(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}
	action := parts[3]

	if action == "participants" && s.mode == card {
		s.mode, s.page = participants, 0
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) field(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}
	action := parts[3]

	if strings.HasPrefix(action, "field") && s.mode == fields {
		field := domain.EventField(strings.TrimPrefix(action, "field"))
		if !validField(field) {
			return navigation.Page{}, nil, errors.New("invalid event field")
		}
		event, err := f.repo.Get(ctx, s.eventID)
		if err != nil {
			return navigation.Page{}, nil, err
		}
		f.clear(userID)
		return navigation.Page{}, &editRequest{event, field}, nil
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) delete(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}
	action := parts[3]

	if action == "delete" && s.mode == card {
		s.mode = confirmDelete
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) keep(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}
	action := parts[3]

	if action == "keep" && s.mode == confirmDelete {
		s.mode = card
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) confirmDelete(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}
	action := parts[3]

	if action == "confirm" && s.mode == confirmDelete {
		if err := f.repo.Delete(ctx, s.eventID); err != nil {
			return navigation.Page{}, nil, err
		}
		f.clear(userID)
		return navigation.Page{Key: "admin:events:deleted", Text: "Мероприятие удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}, nil, nil
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) staleCallback(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, nil, err
	}

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}, nil, nil
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func (f *EventManagementService) list(ctx context.Context, userID int64, callback string) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	s := session{mode: list}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, s)
	if err != nil {
		return navigation.Page{}, nil, err
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, nil, err
	}
	return page, nil, nil
}

func validField(field domain.EventField) bool {
	switch field {
	case domain.EventTitle, domain.EventDescription, domain.EventPhoto, domain.EventLocation, domain.EventDates, domain.EventCapacity, domain.EventTags:
		return true
	}
	return false
}

var conversationStates = [...]fsm.StateID{"admin.events.list", "admin.events.card", "admin.events.fields", "admin.events.delete", "admin.events.participants"}

func reportStorageError(err error) { slog.Error("conversation storage failed", "error", err) }
func (f *EventManagementService) load(ctx context.Context, userID int64) (session, bool, error) {
	state, err := f.machine.CurrentState(ctx, userID)
	if err != nil {
		return session{}, false, err
	}
	value, err := f.machine.GetData(ctx, userID, "session")
	if errors.Is(err, fsm.ErrUserDataNotFound) {
		return session{}, false, nil
	}
	if err != nil {
		return session{}, false, err
	}
	for index, expected := range conversationStates {
		if state == expected {
			value.mode = mode(index)
			return value, true, nil
		}
	}
	return session{}, false, fmt.Errorf("unexpected conversation state %q", state)
}
func (f *EventManagementService) save(ctx context.Context, userID int64, value session) error {
	index := int(value.mode)
	if index < 0 || index >= len(conversationStates) {
		return errors.New("invalid conversation step")
	}
	if err := f.machine.SetData(ctx, userID, "session", value); err != nil {
		return err
	}
	return f.machine.Transition(ctx, userID, conversationStates[index])
}
func (f *EventManagementService) clear(userID int64) {
	if err := f.machine.Finish(context.Background(), userID); err != nil {
		reportStorageError(err)
	}
}

func (f *EventManagementService) render(ctx context.Context, userID int64, s session) (navigation.Page, error) {
	page := navigation.Page{Key: fmt.Sprintf("admin:events:%d:%d", s.mode, s.eventID), Admin: true}
	if s.mode == list {
		page.Key, page.Text = "admin:events:list", fmt.Sprintf("Мероприятия\nСтраница %d\nВыберите мероприятие.", s.page+1)
		events, err := f.repo.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return navigation.Page{}, err
		}
		more := len(events) > pageSize
		if more {
			events = events[:pageSize]
		}
		if len(events) == 0 {
			page.Text = "Мероприятий пока нет."
		}
		page.Keyboard = eventsKeyboard(s, events, more)
	} else {
		event, err := f.repo.Get(ctx, s.eventID)
		if err != nil {
			return navigation.Page{}, err
		}
		switch s.mode {
		case card:
			location, err := time.LoadLocation("Asia/Yekaterinburg")
			if err != nil {
				return navigation.Page{}, err
			}
			date := func(t time.Time) string {
				if t.IsZero() {
					return "не указано"
				}
				return t.In(location).Format("02.01.2006 15:04")
			}
			capacity := "без ограничений"
			if event.Capacity != nil {
				capacity = strconv.Itoa(int(*event.Capacity))
			}
			page.Text = fmt.Sprintf("%s (№%d)\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nЗарегистрировано: %d\nМест: %s\nТеги: %s", event.Title, event.ID, event.Description, event.Location, date(event.StartsAt), date(event.EndsAt), event.RegistrationCount, capacity, strings.Join(event.Tags, "; "))
			page.PhotoFileID = event.PhotoFileID
			page.Keyboard = cardKeyboard(s, event)
		case participants:
			users, err := f.users.ListForEvent(ctx, s.eventID, pageSize+1, s.page*pageSize)
			if err != nil {
				return navigation.Page{}, err
			}
			more := len(users) > pageSize
			if more {
				users = users[:pageSize]
			}
			page.Text = fmt.Sprintf("Участники «%s» (№%d)\nЗарегистрировано: %d\nСтраница %d\n\n%s", event.Title, event.ID, event.RegistrationCount, s.page+1, formatUsers(users, s.page*pageSize, f.adminIDs))
			page.Keyboard = participantsKeyboard(s, more)
		case fields:
			page.Text = fmt.Sprintf("Редактирование «%s» (№%d)\nЧто изменить?", event.Title, event.ID)
			page.Keyboard = fieldsKeyboard(s)
		case confirmDelete:
			page.Text = fmt.Sprintf("Удалить «%s» (№%d)?\nБудут также удалены все регистрации на мероприятие (сейчас: %d).", event.Title, event.ID, event.RegistrationCount)
			page.Keyboard = deleteKeyboard(s)
		}
	}
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		s.token = rand.Text()
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		page, err := f.render(dbCtx, userID, s)
		if err != nil {
			return navigation.Page{}, err
		}
		if err := f.save(ctx, userID, s); err != nil {
			return navigation.Page{}, err
		}
		return page, nil
	}
	return page, nil
}
