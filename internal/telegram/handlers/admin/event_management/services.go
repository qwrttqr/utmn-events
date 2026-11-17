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

const (
	list          fsm.StateID = "admin.events.list"
	card          fsm.StateID = "admin.events.card"
	fields        fsm.StateID = "admin.events.fields"
	confirmDelete fsm.StateID = "admin.events.delete"
	participants  fsm.StateID = "admin.events.participants"
)
const pageSize = pagination.Size

type session struct {
	token   string
	page    int
	eventID int64
}

type conversation struct {
	session
	state fsm.StateID
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
		return f.list(ctx, userID)
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

	if strings.HasPrefix(action, "page") && (s.state == list || s.state == participants) {
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
		s.eventID, s.state = id, card
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

	if action == "edit" && s.state == card {
		s.state = fields
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

	if action == "participants" && s.state == card {
		s.state, s.page = participants, 0
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

	if strings.HasPrefix(action, "field") && s.state == fields {
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

	if action == "delete" && s.state == card {
		s.state = confirmDelete
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

	if action == "keep" && s.state == confirmDelete {
		s.state = card
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

	if action == "confirm" && s.state == confirmDelete {
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

func (f *EventManagementService) list(ctx context.Context, userID int64) (navigation.Page, *editRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	s := conversation{state: list, session: session{}}

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

func reportStorageError(err error) { slog.Error("conversation storage failed", "error", err) }
func (f *EventManagementService) load(ctx context.Context, userID int64) (conversation, bool, error) {
	state, err := f.machine.CurrentState(ctx, userID)
	if err != nil {
		return conversation{}, false, err
	}
	if state == "idle" {
		return conversation{}, false, nil
	}
	switch state {
	case list, card, fields, confirmDelete, participants:
	default:
		return conversation{}, false, fmt.Errorf("unexpected conversation state %q", state)
	}
	value, err := f.machine.GetData(ctx, userID, "session")
	if errors.Is(err, fsm.ErrUserDataNotFound) {
		return conversation{}, false, nil
	}
	if err != nil {
		return conversation{}, false, err
	}
	return conversation{session: value, state: state}, true, nil
}
func (f *EventManagementService) save(ctx context.Context, userID int64, value conversation) error {
	switch value.state {
	case list, card, fields, confirmDelete, participants:
	default:
		return fmt.Errorf("invalid conversation state %q", value.state)
	}
	if err := f.machine.SetData(ctx, userID, "session", value.session); err != nil {
		return err
	}
	return f.machine.Transition(ctx, userID, value.state)
}
func (f *EventManagementService) clear(userID int64) {
	if err := f.machine.Finish(context.Background(), userID); err != nil {
		reportStorageError(err)
	}
}

func (f *EventManagementService) render(ctx context.Context, userID int64, s conversation) (navigation.Page, error) {
	page := navigation.Page{Key: fmt.Sprintf("admin:events:%s:%d", s.state, s.eventID), Admin: true}
	if s.state == list {
		page.Key, page.Text = "admin:events:list", "Мероприятия\nВыберите мероприятие."
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
		switch s.state {
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
			page.Text = fmt.Sprintf("Участники «%s» (№%d)\nЗарегистрировано: %d\n\n%s", event.Title, event.ID, event.RegistrationCount, formatUsers(users, s.page*pageSize, f.adminIDs))
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
