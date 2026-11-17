package tags

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
	"unicode"
	"unicode/utf8"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/navigation"
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

type mode int

const (
	browse mode = iota
	createName
	chooseEvent
	editTags
)

const pageSize = 8

type session struct {
	token    string
	mode     mode
	page     int
	event    domain.EventSummary
	selected map[int64]bool
	tags     []domain.Tag
	events   []domain.EventSummary
}

type button struct{ text, data string }
type screen struct {
	text      string
	buttons   []button
	completed bool
}

type TagsService struct {
	mu      sync.Mutex
	machine *fsm.FSM[string, session]
	repo    Repository
	events  EventRepository
}

func NewService(repo Repository, events EventRepository) *TagsService {
	return &TagsService{machine: fsm.NewInMemoryFSM[string, session]("idle"), repo: repo, events: events}
}

func (f *TagsService) Cancel(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clear(id)
}

func (f *TagsService) enteringName(id int64) bool {
	ctx := context.Background()
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		reportStorageError(err)
		return false
	}
	return exists && s.mode == createName
}

func (f *TagsService) begin(ctx context.Context, id int64) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := session{token: rand.Text(), selected: make(map[int64]bool)}
	view, err := f.render(ctx, &s)
	if err != nil {
		f.clear(id)
		return screen{text: "Не удалось загрузить теги.", buttons: []button{{"Повторить", "admin:tags"}, {"В меню", "admin:menu"}}}, err
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) process(ctx context.Context, id int64, text, callback string) (screen, error) {
	if callback == "" {
		return f.name(ctx, id, text, callback)
	}
	parts := strings.Split(callback, ":")
	if len(parts) != 4 {
		return f.staleCallback(ctx, id, text, callback)
	}
	action := parts[3]
	if action == "create" {
		return f.create(ctx, id, text, callback)
	}
	if action == "assign" {
		return f.assign(ctx, id, text, callback)
	}
	if strings.HasPrefix(action, "page") {
		return f.changePage(ctx, id, text, callback)
	}
	if strings.HasPrefix(action, "event") {
		return f.selectEvent(ctx, id, text, callback)
	}
	if strings.HasPrefix(action, "toggle") {
		return f.toggleTag(ctx, id, text, callback)
	}
	if action == "clear" {
		return f.clearTags(ctx, id, text, callback)
	}
	if action == "save" {
		return f.saveTags(ctx, id, text, callback)
	}
	return f.staleCallback(ctx, id, text, callback)
}

func (f *TagsService) create(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}
	if action == "create" {
		s.mode = createName
	}

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) assign(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}
	if action == "assign" {
		s.mode, s.page = chooseEvent, 0
	}

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) changePage(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}
	if strings.HasPrefix(action, "page") {
		page, err := strconv.Atoi(strings.TrimPrefix(action, "page"))
		if err == nil && page >= 0 && (page == s.page-1 || page == s.page+1) {
			s.page = page
		}
	}

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) selectEvent(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}
	if strings.HasPrefix(action, "event") && s.mode == chooseEvent {
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
			s.event, s.mode, s.page = event, editTags, 0
			break
		}
	}

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) toggleTag(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}
	if strings.HasPrefix(action, "toggle") && s.mode == editTags {
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

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) clearTags(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}
	if action == "clear" && s.mode == editTags {
		s.selected = make(map[int64]bool)
	}

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) saveTags(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}
	if action == "save" && s.mode == editTags {
		var ids []int64
		for id := range s.selected {
			ids = append(ids, id)
		}
		if err := f.repo.SetForEvent(ctx, s.event.ID, ids); err != nil {
			return failure(err)
		}
		f.clear(id)
		return screen{text: "Теги мероприятия сохранены.", completed: true, buttons: []button{{"К тегам", "admin:tags"}, {"В меню", "admin:menu"}}}, nil
	}

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) staleCallback(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (f *TagsService) name(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, id)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Откройте раздел тегов заново.", buttons: []button{{"Теги", "admin:tags"}}}, nil
	}
	old := s
	s.selected = maps.Clone(s.selected)
	action := ""
	if callback != "" {
		parts := strings.Split(callback, ":")
		if len(parts) != 4 || parts[2] != s.token {
			return screen{text: "Эта кнопка устарела.", buttons: []button{{"Теги", "admin:tags"}}}, nil
		}
		action = parts[3]
	}
	s.token = rand.Text()
	failure := func(err error) (screen, error) {
		view := screen{text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if action != "" {
			view.buttons = append(view.buttons, old.button("Повторить", action))
		} else {
			view.text += "\nОтправьте название тега повторно."
		}
		view.buttons = append(view.buttons, button{"Отменить", "admin:tags"})
		return view, err
	}
	if callback == "" && s.mode == createName {
		name := strings.ToLower(strings.TrimSpace(text))
		if name == "" || utf8.RuneCountInString(name) > 40 || strings.ContainsAny(name, ";/") || strings.ContainsFunc(name, unicode.IsControl) {
			return screen{text: "Введите один тег: от 1 до 40 символов, без команд, точки с запятой и переносов строк.", buttons: []button{{"Отменить", "admin:tags"}}}, nil
		}
		tag, err := f.repo.Create(ctx, name)
		if err != nil {
			return failure(err)
		}
		s.mode = browse
		if err := f.save(ctx, id, s); err != nil {
			return screen{}, err
		}
		return screen{text: fmt.Sprintf("Тег «%s» сохранён.", tag.Name), completed: true, buttons: []button{
			s.button("Создать ещё", "create"), s.button("Назначить мероприятию", "assign"), {"К тегам", "admin:tags"},
		}}, nil
	}

	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	if err := f.save(ctx, id, s); err != nil {
		return screen{}, err
	}
	return view, nil
}

func (s session) button(text, action string) button {
	return button{text, "admin:tag:" + s.token + ":" + action}
}

var conversationStates = [...]fsm.StateID{"admin.tags.browse", "admin.tags.create", "admin.tags.choose_event", "admin.tags.edit"}

func reportStorageError(err error) { slog.Error("conversation storage failed", "error", err) }
func (f *TagsService) load(ctx context.Context, userID int64) (session, bool, error) {
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
func (f *TagsService) save(ctx context.Context, userID int64, value session) error {
	index := int(value.mode)
	if index < 0 || index >= len(conversationStates) {
		return errors.New("invalid conversation step")
	}
	if err := f.machine.SetData(ctx, userID, "session", value); err != nil {
		return err
	}
	return f.machine.Transition(ctx, userID, conversationStates[index])
}
func (f *TagsService) clear(userID int64) {
	if err := f.machine.Finish(context.Background(), userID); err != nil {
		reportStorageError(err)
	}
}

func (f *TagsService) render(ctx context.Context, s *session) (screen, error) {
	view := screen{}
	if s.mode == createName {
		return screen{text: "Введите название нового тега.", buttons: []button{{"Отменить", "admin:tags"}}}, nil
	}
	more := false
	if s.mode == chooseEvent {
		events, err := f.events.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return screen{}, err
		}
		more = len(events) > pageSize
		if more {
			events = events[:pageSize]
		}
		s.events = events
		view.text = "Выберите мероприятие для назначения тегов."
		if len(events) == 0 {
			view.text = "Мероприятий пока нет."
		}
		for _, event := range events {
			label := []rune(event.Title)
			if len(label) > 45 {
				label = append(label[:45], '…')
			}
			view.buttons = append(view.buttons, s.button(fmt.Sprintf("№%d %s", event.ID, string(label)), "event"+strconv.FormatInt(event.ID, 10)))
		}
	} else {
		tags, err := f.repo.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return screen{}, err
		}
		more = len(tags) > pageSize
		if more {
			tags = tags[:pageSize]
		}
		s.tags = tags
		if s.mode == editTags {
			view.text = fmt.Sprintf("Теги мероприятия №%d\nНажимайте на теги для выбора. Выбрано: %d.\nИзменения применяются после сохранения.", s.event.ID, len(s.selected))
			for _, tag := range tags {
				mark := "○ "
				if s.selected[tag.ID] {
					mark = "✓ "
				}
				view.buttons = append(view.buttons, s.button(mark+tag.Name, "toggle"+strconv.FormatInt(tag.ID, 10)))
			}
		} else {
			var names []string
			for _, tag := range tags {
				names = append(names, "• "+tag.Name)
			}
			view.text = "Теги\n\n" + strings.Join(names, "\n")
		}
		if len(tags) == 0 {
			if s.mode == browse {
				view.text += "\nТегов пока нет. Нажмите «Создать тег»."
			} else {
				view.text += "\nТегов пока нет. Вернитесь назад и создайте тег."
			}
		}
	}
	if s.page > 0 {
		view.buttons = append(view.buttons, s.button("⬅️", "page"+strconv.Itoa(s.page-1)))
	}
	if more {
		view.buttons = append(view.buttons, s.button("➡️", "page"+strconv.Itoa(s.page+1)))
	}
	if s.mode == browse {
		view.buttons = append(view.buttons, s.button("Создать тег", "create"), s.button("Назначить теги мероприятию", "assign"))
	}
	if s.mode == editTags {
		view.buttons = append(view.buttons, s.button("Снять все теги", "clear"), s.button("Сохранить", "save"))
	}
	return view, nil
}

func cloneSession(s session) session {
	s.selected = maps.Clone(s.selected)
	s.tags = slices.Clone(s.tags)
	s.events = slices.Clone(s.events)
	return s
}

func (f *TagsService) page(userID int64, view screen, snapshot *session) navigation.Page {
	page := navigation.Page{Key: "admin:tags:result", Text: view.text, Keyboard: keyboard(view), Admin: true, ClearHistory: view.completed || snapshot == nil}
	if snapshot == nil {
		return page
	}
	saved := cloneSession(*snapshot)
	page.Key = fmt.Sprintf("admin:tags:%d:%d", saved.mode, saved.event.ID)
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		s := cloneSession(saved)
		s.token = rand.Text()
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		view, err := f.render(dbCtx, &s)
		if err != nil {
			return navigation.Page{}, err
		}
		if err := f.save(ctx, userID, s); err != nil {
			return navigation.Page{}, err
		}
		return f.page(userID, view, &s), nil
	}
	return page
}

func (f *TagsService) snapshot(ctx context.Context, userID int64) (*session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists, err := f.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	saved := cloneSession(s)
	return &saved, nil
}
