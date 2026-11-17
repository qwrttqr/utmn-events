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

const (
	firstName    fsm.StateID = "registration.first_name"
	lastName     fsm.StateID = "registration.last_name"
	surname      fsm.StateID = "registration.surname"
	confirmation fsm.StateID = "registration.confirmation"
)

type draft struct {
	profile domain.User
}

type conversation struct {
	draft
	state fsm.StateID
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

func (s *RegistrationService) inState(userID int64, expected fsm.StateID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.machine.CurrentState(context.Background(), userID)
	if err != nil {
		reportStorageError(err)
		return false
	}
	return state == expected
}

func (s *RegistrationService) begin(ctx context.Context, userID int64, username string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := conversation{state: firstName, draft: draft{profile: domain.User{TelegramID: userID, Username: username, Role: "user"}}}
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
	if d.state != firstName {
		return prompt(d), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := prompt(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.FirstName, d.state = text, lastName
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
	if d.state != lastName {
		return prompt(d), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := prompt(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.LastName, d.state = text, surname
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
	if d.state != surname {
		return prompt(d), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := prompt(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.Surname, d.state = text, confirmation
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
	if d.state != surname {
		return prompt(d), nil
	}
	d.profile.Surname, d.state = "", confirmation
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
	if d.state != confirmation {
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
		d = conversation{state: firstName, draft: draft{profile: domain.User{TelegramID: userID, Username: username, Role: "user"}}}
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

func prompt(d conversation) screen {
	switch d.state {
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

func reportStorageError(err error) { slog.Error("conversation storage failed", "error", err) }
func (f *RegistrationService) load(ctx context.Context, userID int64) (conversation, bool, error) {
	state, err := f.machine.CurrentState(ctx, userID)
	if err != nil {
		return conversation{}, false, err
	}
	if state == "idle" {
		return conversation{}, false, nil
	}
	switch state {
	case firstName, lastName, surname, confirmation:
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
	return conversation{draft: value, state: state}, true, nil
}
func (f *RegistrationService) save(ctx context.Context, userID int64, value conversation) error {
	switch value.state {
	case firstName, lastName, surname, confirmation:
	default:
		return fmt.Errorf("invalid conversation state %q", value.state)
	}
	if err := f.machine.SetData(ctx, userID, "session", value.draft); err != nil {
		return err
	}
	return f.machine.Transition(ctx, userID, value.state)
}
func (p *RegistrationService) clear(userID int64) {
	if err := p.machine.Finish(context.Background(), userID); err != nil {
		reportStorageError(err)
	}
}
