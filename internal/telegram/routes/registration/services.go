package registration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
)

const (
	firstName                fsm.StateID = "registration.first_name"
	lastName                 fsm.StateID = "registration.last_name"
	surname                  fsm.StateID = "registration.surname"
	registrationConfirmation fsm.StateID = "registration.confirmation"
)

type draft struct {
	profile domain.User
}

var errRegistrationMissing = errors.New("registration draft missing")

const invalidNameMessage = "Введите от 1 до 100 символов без команд и переносов строк."

type registrationResult struct {
	state   fsm.StateID
	draft   draft
	message string
}

type UserRepository interface {
	RegistrationStatus(ctx context.Context, telegramID int64) (registered, admin bool, err error)
	Create(ctx context.Context, user domain.User) error
}

type Service struct {
	users   UserRepository
	machine *fsm.FSM
}

func NewService(machine *fsm.FSM, users UserRepository) *Service {
	return &Service{
		users:   users,
		machine: machine,
	}
}

func (s *Service) Cancel(userID int64) {
	s.clear(userID)
}

func (s *Service) hasDraft(userID int64) bool {
	_, _, exists := fsm.GetDraft[draft](s.machine, userID)
	return exists
}

func (s *Service) inState(userID int64, expected fsm.StateID) bool {
	return s.machine.CurrentState(userID) == expected
}

func (s *Service) begin(userID int64, username string) {
	d := draft{profile: domain.User{TelegramID: userID, Username: username, Role: "user"}}
	s.machine.Replace(userID, firstName, d)
}

func (s *Service) setFirstName(userID int64, text string) registrationResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)

	if !exists {
		return registrationResult{state: fsm.Idle}
	}

	text = strings.TrimSpace(text)

	if !validName(text) {
		return registrationResult{state: state, draft: d, message: invalidNameMessage}
	}

	d.profile.FirstName = text
	s.machine.Replace(userID, lastName, d)
	return registrationResult{state: lastName, draft: d}
}

func (s *Service) setLastName(userID int64, text string) registrationResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)

	if !exists {
		return registrationResult{state: fsm.Idle}
	}

	text = strings.TrimSpace(text)

	if !validName(text) {
		return registrationResult{state: state, draft: d, message: invalidNameMessage}
	}

	d.profile.LastName = text
	s.machine.Replace(userID, surname, d)
	return registrationResult{state: surname, draft: d}
}

func (s *Service) setSurname(userID int64, text string) registrationResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists {
		return registrationResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		return registrationResult{state: state, draft: d, message: invalidNameMessage}
	}
	d.profile.Surname = text
	s.machine.Replace(userID, registrationConfirmation, d)
	return registrationResult{state: registrationConfirmation, draft: d}
}

func (s *Service) skipSurname(userID int64) registrationResult {
	_, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists {
		return registrationResult{state: fsm.Idle}
	}
	d.profile.Surname = ""
	s.machine.Replace(userID, registrationConfirmation, d)
	return registrationResult{state: registrationConfirmation, draft: d}
}

func (s *Service) confirm(ctx context.Context, userID int64, username string) (registrationResult, error) {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists {
		return registrationResult{state: fsm.Idle}, errRegistrationMissing
	}
	d.profile.Username = username
	if err := s.users.Create(ctx, d.profile); err != nil {
		return registrationResult{state: state, draft: d}, fmt.Errorf("save registration: %w", err)
	}
	s.clear(userID)
	return registrationResult{state: fsm.Idle}, nil
}

func (s *Service) preview(userID int64, username string) registrationResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
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

func (s *Service) clear(userID int64) {
	switch s.machine.CurrentState(userID) {
	case firstName, lastName, surname, registrationConfirmation:
		s.machine.Clear(userID)
	}
}
