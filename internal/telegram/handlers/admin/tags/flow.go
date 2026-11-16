package tags

import (
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"utmn-events-bot/internal/domain"
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

type Flow struct {
	mu       sync.Mutex
	sessions map[int64]session
	repo     Repository
	events   EventRepository
}

func New(repo Repository, events EventRepository) *Flow {
	return &Flow{sessions: make(map[int64]session), repo: repo, events: events}
}

func (f *Flow) Cancel(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, id)
}

func (f *Flow) enteringName(id int64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists := f.sessions[id]
	return exists && s.mode == createName
}

func (f *Flow) begin(ctx context.Context, id int64) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := session{token: rand.Text(), selected: make(map[int64]bool)}
	view, err := f.render(ctx, &s)
	if err != nil {
		delete(f.sessions, id)
		return screen{text: "Не удалось загрузить теги.", buttons: []button{{"Повторить", "admin:tags"}, {"В меню", "admin:menu"}}}, err
	}
	f.sessions[id] = s
	return view, nil
}

func (f *Flow) process(ctx context.Context, id int64, text, callback string) (screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, exists := f.sessions[id]
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
	// Each screen gets fresh callbacks, so an old Save button cannot affect a
	// different event or a newer selection in the same session.
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
		f.sessions[id] = s
		return screen{text: fmt.Sprintf("Тег «%s» сохранён.", tag.Name), completed: true, buttons: []button{
			s.button("Создать ещё", "create"), s.button("Назначить мероприятию", "assign"), {"К тегам", "admin:tags"},
		}}, nil
	}
	switch {
	case action == "create":
		s.mode = createName
	case action == "assign":
		s.mode, s.page = chooseEvent, 0
	case strings.HasPrefix(action, "page"):
		page, err := strconv.Atoi(strings.TrimPrefix(action, "page"))
		// Only adjacent pages can be requested; bound offsets from forged callbacks.
		if err != nil || page < 0 || (page != s.page-1 && page != s.page+1) {
			break
		}
		s.page = page
	case strings.HasPrefix(action, "event") && s.mode == chooseEvent:
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
	case strings.HasPrefix(action, "toggle") && s.mode == editTags:
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
	case action == "clear" && s.mode == editTags:
		s.selected = make(map[int64]bool)
	case action == "save" && s.mode == editTags:
		var ids []int64
		for id := range s.selected {
			ids = append(ids, id)
		}
		if err := f.repo.SetForEvent(ctx, s.event.ID, ids); err != nil {
			return failure(err)
		}
		delete(f.sessions, id)
		return screen{text: "Теги мероприятия сохранены.", completed: true, buttons: []button{{"К тегам", "admin:tags"}, {"В меню", "admin:menu"}}}, nil
	}
	view, err := f.render(ctx, &s)
	if err != nil {
		return failure(err)
	}
	f.sessions[id] = s
	return view, nil
}

func (s session) button(text, action string) button {
	return button{text, "admin:tag:" + s.token + ":" + action}
}
