package registration

import (
	"context"
	"fmt"
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

type registrationStatus uint8

const (
	registrationMissing registrationStatus = iota + 1
	registrationCancelled
	registrationCompleted
	registrationInvalidName
)

type registrationResult struct {
	state  fsm.StateID
	draft  draft
	status registrationStatus
}

type UserRepository interface {
	IsRegistered(ctx context.Context, telegramID int64) (bool, error)
	Create(ctx context.Context, user domain.User) error
}

type RegistrationService struct {
	mu      sync.Mutex
	users   UserRepository
	machine *fsm.FSM
}

func NewService(machine *fsm.FSM, users UserRepository) *RegistrationService {
	return &RegistrationService{
		users:   users,
		machine: machine,
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
	_, _, exists := fsm.Load[draft](s.machine, userID)
	return exists
}

func (s *RegistrationService) inState(userID int64, expected fsm.StateID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.machine.CurrentState(userID) == expected
}

func (s *RegistrationService) begin(userID int64, username string) registrationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := draft{profile: domain.User{TelegramID: userID, Username: username, Role: "user"}}
	s.machine.Replace(userID, firstName, d)
	return registrationResult{state: firstName, draft: d}
}

func (s *RegistrationService) setFirstName(userID int64, text string) registrationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, d, exists := fsm.Load[draft](s.machine, userID)
	if !exists {
		return registrationResult{status: registrationMissing}
	}
	if state != firstName {
		return registrationResult{state: state, draft: d}
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		return registrationResult{state: state, draft: d, status: registrationInvalidName}
	}
	d.profile.FirstName, state = text, lastName
	s.machine.Replace(userID, state, d)
	return registrationResult{state: state, draft: d}
}

func (s *RegistrationService) setLastName(userID int64, text string) registrationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, d, exists := fsm.Load[draft](s.machine, userID)
	if !exists {
		return registrationResult{status: registrationMissing}
	}
	if state != lastName {
		return registrationResult{state: state, draft: d}
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		return registrationResult{state: state, draft: d, status: registrationInvalidName}
	}
	d.profile.LastName, state = text, surname
	s.machine.Replace(userID, state, d)
	return registrationResult{state: state, draft: d}
}

func (s *RegistrationService) setSurname(userID int64, text string) registrationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, d, exists := fsm.Load[draft](s.machine, userID)
	if !exists {
		return registrationResult{status: registrationMissing}
	}
	if state != surname {
		return registrationResult{state: state, draft: d}
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		return registrationResult{state: state, draft: d, status: registrationInvalidName}
	}
	d.profile.Surname, state = text, confirmation
	s.machine.Replace(userID, state, d)
	return registrationResult{state: state, draft: d}
}

func (s *RegistrationService) skipSurname(userID int64) registrationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, d, exists := fsm.Load[draft](s.machine, userID)
	if !exists {
		return registrationResult{status: registrationMissing}
	}
	if state != surname {
		return registrationResult{state: state, draft: d}
	}
	d.profile.Surname, state = "", confirmation
	s.machine.Replace(userID, state, d)
	return registrationResult{state: state, draft: d}
}

func (s *RegistrationService) confirm(ctx context.Context, userID int64, username string) (registrationResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, d, exists := fsm.Load[draft](s.machine, userID)
	if !exists {
		return registrationResult{status: registrationMissing}, nil
	}
	if state != confirmation {
		return registrationResult{state: state, draft: d}, nil
	}
	d.profile.Username = username
	if err := s.users.Create(ctx, d.profile); err != nil {
		return registrationResult{state: state, draft: d}, fmt.Errorf("save registration: %w", err)
	}
	s.clear(userID)
	return registrationResult{status: registrationCompleted}, nil
}

func (s *RegistrationService) cancelDraft(userID int64) registrationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clear(userID)
	return registrationResult{status: registrationCancelled}
}

func (s *RegistrationService) preview(userID int64, username string) registrationResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, d, exists := fsm.Load[draft](s.machine, userID)
	if !exists {
		state = firstName
		d = draft{profile: domain.User{TelegramID: userID, Username: username, Role: "user"}}
		s.machine.Replace(userID, state, d)
	}
	return registrationResult{state: state, draft: d}
}
func validName(value string) bool {
	return value != "" && utf8.RuneCountInString(value) <= 100 &&
		!strings.HasPrefix(value, "/") && !strings.ContainsFunc(value, unicode.IsControl)
}

func (s *RegistrationService) clear(userID int64) {
	switch s.machine.CurrentState(userID) {
	case firstName, lastName, surname, confirmation:
		s.machine.Clear(userID)
	}
}
