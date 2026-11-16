package registration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"utmn-events-bot/internal/domain"
)

type step int

const (
	firstName step = iota
	lastName
	surname
	confirmation
)

type draft struct {
	profile domain.User
	step    step
}

type screen struct {
	text      string
	confirm   bool
	skip      bool
	cancel    bool
	start     bool
	menu      bool
	temporary bool
}

type UserRepository interface {
	IsRegistered(ctx context.Context, telegramID int64) (bool, error)
	Create(ctx context.Context, user domain.User) error
}

type Pipeline struct {
	mu     sync.Mutex
	users  UserRepository
	drafts map[int64]draft
}

func New(users UserRepository) *Pipeline {
	return &Pipeline{
		users:  users,
		drafts: make(map[int64]draft),
	}
}

// process returns handled=false only when the update may reach normal handlers.
func (p *Pipeline) process(ctx context.Context, userID int64, username, text, callback string) (screen, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	registered, err := p.users.IsRegistered(ctx, userID)
	if err != nil {
		return screen{}, true, fmt.Errorf("load registration status: %w", err)
	}
	if registered {
		delete(p.drafts, userID)
		if strings.HasPrefix(callback, "registration:") {
			return screen{text: "Вы уже зарегистрированы.", menu: true}, true, nil
		}
		return screen{}, false, nil
	}

	text = strings.TrimSpace(text)
	if text == "/cancel" || callback == "registration:cancel" {
		delete(p.drafts, userID)
		return screen{text: "Регистрация отменена. Для доступа к боту необходимо зарегистрироваться.", start: true}, true, nil
	}

	d, exists := p.drafts[userID]
	if !exists || text == "/start" || callback == "registration:start" || callback == "registration:restart" {
		d = draft{profile: domain.User{TelegramID: userID, Username: username, Role: "user"}, step: firstName}
		p.drafts[userID] = d
		return prompt(d), true, nil
	}

	if callback != "" {
		if callback == "registration:confirm" && d.step == confirmation {
			d.profile.Username = username
			if err := p.users.Create(ctx, d.profile); err != nil {
				return prompt(d), true, fmt.Errorf("save registration: %w", err)
			}
			delete(p.drafts, userID)
			return screen{text: "Регистрация завершена!", menu: true}, true, nil
		}
		if callback == "registration:skip" && d.step == surname {
			d.profile.Surname = ""
			d.step = confirmation
			p.drafts[userID] = d
			return prompt(d), true, nil
		}
		return prompt(d), true, nil
	}

	if d.step == confirmation {
		return prompt(d), true, nil
	}
	if d.step == surname && text == "/skip" {
		d.profile.Surname = ""
	} else {
		if !validName(text) {
			view := prompt(d)
			view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
			return view, true, nil
		}
		switch d.step {
		case firstName:
			d.profile.FirstName = text
		case lastName:
			d.profile.LastName = text
		case surname:
			d.profile.Surname = text
		}
	}
	d.step++
	p.drafts[userID] = d
	return prompt(d), true, nil
}

func validName(value string) bool {
	return value != "" && utf8.RuneCountInString(value) <= 100 &&
		!strings.HasPrefix(value, "/") && !strings.ContainsFunc(value, unicode.IsControl)
}

func prompt(d draft) screen {
	switch d.step {
	case firstName:
		return screen{text: "Для доступа к боту необходимо зарегистрироваться.\nВведите ваше имя.", cancel: true}
	case lastName:
		return screen{text: "Введите вашу фамилию.", cancel: true}
	case surname:
		return screen{text: "Введите ваше отчество или нажмите «Пропустить».", skip: true, cancel: true}
	default:
		name := d.profile.Surname
		if name == "" {
			name = "не указано"
		}
		return screen{
			text:    fmt.Sprintf("Проверьте данные:\nИмя: %s\nФамилия: %s\nОтчество: %s", d.profile.FirstName, d.profile.LastName, name),
			confirm: true,
			cancel:  true,
		}
	}
}
