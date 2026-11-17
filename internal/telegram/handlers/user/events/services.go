package events

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
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

type EventsService struct {
	mu            sync.Mutex
	machine       *fsm.FSM[string, session]
	repo          Repository
	tags          TagRepository
	registrations RegistrationRepository
	location      *time.Location
	now           func() time.Time
}

func NewService(repo Repository, tags TagRepository, registrations RegistrationRepository) (*EventsService, error) {
	location, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		return nil, err
	}
	return &EventsService{machine: fsm.NewInMemoryFSM[string, session]("idle"), repo: repo, tags: tags, registrations: registrations, location: location, now: time.Now}, nil
}

func (f *EventsService) Cancel(id int64) { f.mu.Lock(); defer f.mu.Unlock(); f.clear(id) }

func cloneSession(s session) session {
	s.selected = maps.Clone(s.selected)
	s.tags = slices.Clone(s.tags)
	return s
}

func (f *EventsService) process(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	if callback == "user:events" {
		return f.browse(ctx, userID, callback)
	}
	if callback == "user:registrations" {
		return f.myRegistrations(ctx, userID, callback)
	}
	if callback == "user:search" {
		return f.chooseTags(ctx, userID, callback)
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
	return f.staleCallback(ctx, userID, callback)
}

func (f *EventsService) page(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if strings.HasPrefix(action, "page") && (s.mode == all || s.mode == mine || s.mode == results || s.mode == chooseTags) {
		if page, ok := pagination.Move(s.page, action); ok {
			s.page = page
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
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

var conversationStates = [...]fsm.StateID{"user.events.all", "user.events.mine", "user.events.choose_tags", "user.events.results", "user.events.card", "user.events.cancel"}

func (f *EventsService) open(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if strings.HasPrefix(action, "open") {
		id, err := strconv.ParseInt(strings.TrimPrefix(action, "open"), 10, 32)
		if err != nil || id <= 0 {
			return navigation.Page{}, errors.New("invalid event ID")
		}
		s.eventID, s.mode = id, card
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) toggleTag(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if strings.HasPrefix(action, "toggle") && s.mode == chooseTags {
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
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) clearTags(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if action == "clear" && s.mode == chooseTags {
		s.selected = make(map[int64]string)
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) search(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if action == "search" && s.mode == chooseTags {
		if len(s.selected) == 0 {
			notice = "Выберите хотя бы один тег."
		} else {
			s.mode, s.page = results, 0
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
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) enroll(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if action == "enroll" && s.mode == card {
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
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) cancelEnrollment(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if action == "cancel" && s.mode == card {
		s.mode = cancelEnrollment
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) keepEnrollment(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if action == "keep" && s.mode == cancelEnrollment {
		s.mode = card
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) confirmCancellation(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}
	action := parts[3]

	if action == "confirm" && s.mode == cancelEnrollment {
		if err := f.registrations.Unenroll(ctx, s.eventID, userID); err != nil {
			return navigation.Page{}, err
		}
		s.mode, notice = card, "Запись на мероприятие отменена."
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) staleCallback(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return navigation.Page{}, err
	}
	s = cloneSession(s)
	notice := ""

	parts := strings.Split(callback, ":")
	if !exists || len(parts) != 4 || parts[2] != s.token {
		return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}, nil
	}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) browse(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	notice := ""

	s := session{mode: all, selected: make(map[int64]string)}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) myRegistrations(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	notice := ""

	s := session{mode: mine, selected: make(map[int64]string)}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func (f *EventsService) chooseTags(ctx context.Context, userID int64, callback string) (navigation.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	notice := ""

	s := session{mode: chooseTags, selected: make(map[int64]string)}

	s.token = rand.Text()
	page, err := f.render(ctx, userID, &s)
	if err != nil {
		return navigation.Page{}, err
	}
	if notice != "" {
		page.Text = notice + "\n\n" + page.Text
	}
	if err := f.save(ctx, userID, s); err != nil {
		return navigation.Page{}, err
	}
	return page, nil
}

func reportStorageError(err error) { slog.Error("conversation storage failed", "error", err) }
func (f *EventsService) load(ctx context.Context, userID int64) (session, bool, error) {
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
func (f *EventsService) save(ctx context.Context, userID int64, value session) error {
	index := int(value.mode)
	if index < 0 || index >= len(conversationStates) {
		return errors.New("invalid conversation step")
	}
	if err := f.machine.SetData(ctx, userID, "session", value); err != nil {
		return err
	}
	return f.machine.Transition(ctx, userID, conversationStates[index])
}
func (f *EventsService) clear(userID int64) {
	if err := f.machine.Finish(context.Background(), userID); err != nil {
		reportStorageError(err)
	}
}

func (f *EventsService) render(ctx context.Context, userID int64, s *session) (navigation.Page, error) {
	page := navigation.Page{Key: fmt.Sprintf("user:events:%d:%d", s.mode, s.eventID)}
	switch s.mode {
	case chooseTags:
		tags, err := f.tags.List(ctx, pagination.Size+1, s.page*pagination.Size)
		if err != nil {
			return navigation.Page{}, err
		}
		more := len(tags) > pagination.Size
		if more {
			tags = tags[:pagination.Size]
		}
		s.tags = tags
		page.Text = fmt.Sprintf("Выберите хотя бы один тег.\nСовпадение по любому выбранному тегу.\nВыбрано: %d\nСтраница %d", len(s.selected), s.page+1)
		if len(tags) == 0 {
			page.Text += "\nТегов пока нет."
		}
		page.Keyboard = tagsKeyboard(*s, tags, more)
	case all, mine, results:
		filter := domain.EventFilter{UpcomingOnly: s.mode != mine}
		label := "Предстоящие мероприятия"
		if s.mode == mine {
			filter.UserTelegramID, label = userID, "Мои записи"
		}
		if s.mode == results {
			if len(s.selected) == 0 {
				return navigation.Page{}, fmt.Errorf("tag search requires at least one tag")
			}
			var names []string
			for id, name := range s.selected {
				filter.TagIDs = append(filter.TagIDs, id)
				names = append(names, name)
			}
			slices.Sort(filter.TagIDs)
			slices.Sort(names)
			label = "Мероприятия по тегам: " + strings.Join(names, "; ")
		}
		events, err := f.repo.Browse(ctx, filter, pagination.Size+1, s.page*pagination.Size)
		if err != nil {
			return navigation.Page{}, err
		}
		more := len(events) > pagination.Size
		if more {
			events = events[:pagination.Size]
		}
		page.Text = fmt.Sprintf("%s\nСтраница %d", label, s.page+1)
		if len(events) == 0 {
			page.Text += "\nМероприятий не найдено."
		}
		page.Keyboard = eventsKeyboard(*s, events, more)
	case card, cancelEnrollment:
		event, err := f.repo.Get(ctx, s.eventID)
		if err != nil {
			return navigation.Page{}, err
		}
		enrolled, err := f.registrations.IsEnrolled(ctx, s.eventID, userID)
		if err != nil {
			return navigation.Page{}, err
		}
		page.Keyboard = cardKeyboard(*s, event, enrolled, f.now())
		if s.mode == cancelEnrollment {
			page.Text = fmt.Sprintf("Отменить вашу запись на «%s» (№%d)?", event.Title, event.ID)

		} else {
			date := func(value time.Time) string {
				if value.IsZero() {
					return "не указано"
				}
				return value.In(f.location).Format("02.01.2006 15:04")
			}
			capacity := "без ограничений"
			if event.Capacity != nil {
				capacity = strconv.Itoa(int(*event.Capacity))
			}
			page.Text = fmt.Sprintf("%s (№%d)\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nЗаписано: %d\nМест: %s\nТеги: %s", event.Title, event.ID, event.Description, event.Location, date(event.StartsAt), date(event.EndsAt), event.RegistrationCount, capacity, strings.Join(event.Tags, "; "))
			page.PhotoFileID = event.PhotoFileID
			if enrolled {
				page.Text += "\n\nВы записаны на это мероприятие."

			} else if !event.StartsAt.After(f.now()) {
				page.Text += "\n\nЗапись закрыта."
			} else if event.Capacity != nil && event.RegistrationCount >= int64(*event.Capacity) {
				page.Text += "\n\nСвободных мест нет."
			} else {

			}
		}
	}
	saved := cloneSession(*s)
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		copy := cloneSession(saved)
		copy.token = rand.Text()
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		page, err := f.render(dbCtx, userID, &copy)
		if err != nil {
			return navigation.Page{}, err
		}
		if err := f.save(ctx, userID, copy); err != nil {
			return navigation.Page{}, err
		}
		return page, nil
	}
	return page, nil
}
