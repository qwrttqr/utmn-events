package registration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
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

type RegistrationService struct {
	mu      sync.Mutex
	users   UserRepository
	machine *fsm.FSM[string, draft]
}

func NewService(users UserRepository) *RegistrationService {
	return &RegistrationService{
		users:   users,
		machine: fsm.NewInMemoryFSM[string, draft]("idle"),
	}
}

func (s *RegistrationService) isRegistered(ctx context.Context, userID int64) (bool, error) {
	registered, err := s.users.IsRegistered(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("load registration status: %w", err)
	}
	return registered, nil
}

func (s *RegistrationService) Cancel(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clear(userID)
}

func (s *RegistrationService) hasDraft(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists, err := s.load(context.Background(), userID)
	if err != nil {
		reportStorageError(err)
		return false
	}
	return exists
}

func (s *RegistrationService) inState(userID int64, expected step) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.machine.CurrentState(context.Background(), userID)
	if err != nil {
		reportStorageError(err)
		return false
	}
	return state == conversationStates[expected]
}

func (s *RegistrationService) begin(ctx context.Context, userID int64, username string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := draft{profile: domain.User{TelegramID: userID, Username: username, Role: "user"}, step: firstName}
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return prompt(d), nil
}

func (s *RegistrationService) setFirstName(ctx context.Context, userID int64, text string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Начните регистрацию.", start: true}, nil
	}
	if d.step != firstName {
		return prompt(d), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := prompt(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.FirstName, d.step = text, lastName
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return prompt(d), nil
}

func (s *RegistrationService) setLastName(ctx context.Context, userID int64, text string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Начните регистрацию.", start: true}, nil
	}
	if d.step != lastName {
		return prompt(d), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := prompt(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.LastName, d.step = text, surname
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return prompt(d), nil
}

func (s *RegistrationService) setSurname(ctx context.Context, userID int64, text string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Начните регистрацию.", start: true}, nil
	}
	if d.step != surname {
		return prompt(d), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := prompt(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.Surname, d.step = text, confirmation
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return prompt(d), nil
}

func (s *RegistrationService) skipSurname(ctx context.Context, userID int64) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Начните регистрацию.", start: true}, nil
	}
	if d.step != surname {
		return prompt(d), nil
	}
	d.profile.Surname, d.step = "", confirmation
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return prompt(d), nil
}

func (s *RegistrationService) confirm(ctx context.Context, userID int64, username string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		return screen{text: "Начните регистрацию.", start: true}, nil
	}
	if d.step != confirmation {
		return prompt(d), nil
	}
	d.profile.Username = username
	if err := s.users.Create(ctx, d.profile); err != nil {
		return prompt(d), fmt.Errorf("save registration: %w", err)
	}
	s.clear(userID)
	return screen{text: "Регистрация завершена!", menu: true}, nil
}

func (s *RegistrationService) cancelDraft(ctx context.Context, userID int64) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.machine.Finish(ctx, userID); err != nil {
		return screen{}, err
	}
	return screen{text: "Регистрация отменена. Для доступа к боту необходимо зарегистрироваться.", start: true}, nil
}

func (s *RegistrationService) preview(ctx context.Context, userID int64, username string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists {
		d = draft{profile: domain.User{TelegramID: userID, Username: username, Role: "user"}, step: firstName}
		if err := s.save(ctx, userID, d); err != nil {
			return screen{}, err
		}
	}
	return prompt(d), nil
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

var conversationStates = [...]fsm.StateID{"registration.first_name", "registration.last_name", "registration.surname", "registration.confirmation"}

func reportStorageError(err error) { slog.Error("conversation storage failed", "error", err) }
func (p *RegistrationService) load(ctx context.Context, userID int64) (draft, bool, error) {
	state, err := p.machine.CurrentState(ctx, userID)
	if err != nil {
		return draft{}, false, err
	}
	value, err := p.machine.GetData(ctx, userID, "session")
	if errors.Is(err, fsm.ErrUserDataNotFound) {
		return draft{}, false, nil
	}
	if err != nil {
		return draft{}, false, err
	}
	for index, expected := range conversationStates {
		if state == expected {
			value.step = step(index)
			return value, true, nil
		}
	}
	return draft{}, false, fmt.Errorf("unexpected conversation state %q", state)
}
func (p *RegistrationService) save(ctx context.Context, userID int64, value draft) error {
	index := int(value.step)
	if index < 0 || index >= len(conversationStates) {
		return errors.New("invalid conversation step")
	}
	if err := p.machine.SetData(ctx, userID, "session", value); err != nil {
		return err
	}
	return p.machine.Transition(ctx, userID, conversationStates[index])
}
func (p *RegistrationService) clear(userID int64) {
	if err := p.machine.Finish(context.Background(), userID); err != nil {
		reportStorageError(err)
	}
}
