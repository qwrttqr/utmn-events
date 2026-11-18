package profile

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

type Repository interface {
	GetByTelegramID(context.Context, int64) (domain.User, error)
	UpdateFirstName(context.Context, int64, string) error
	UpdateLastName(context.Context, int64, string) error
	UpdateSurname(context.Context, int64, string) error
}

const (
	profileView      fsm.StateID = "user.profile.view"
	profileFirstName fsm.StateID = "user.profile.first_name"
	profileLastName  fsm.StateID = "user.profile.last_name"
	profileSurname   fsm.StateID = "user.profile.surname"
	profilePreview   fsm.StateID = "user.profile.preview"
)

type profileField string

const (
	firstNameField profileField = "first_name"
	lastNameField  profileField = "last_name"
	surnameField   profileField = "surname"
)

type session struct {
	profile domain.User
	field   profileField
}

type profileStatus uint8

const (
	profileStale profileStatus = iota + 1
	profileCancelled
	profileInvalidName
	profileUpdated
)

type profileResult struct {
	state   fsm.StateID
	session session
	status  profileStatus
}

type ProfileService struct {
	repo    Repository
	machine *fsm.FSM
}

func NewService(machine *fsm.FSM, repo Repository) *ProfileService {
	return &ProfileService{repo: repo, machine: machine}
}
func (s *ProfileService) inState(userID int64, expected fsm.StateID) bool {
	return s.machine.CurrentState(userID) == expected
}
func (s *ProfileService) begin(ctx context.Context, userID int64) (profileResult, error) {
	user, err := s.repo.GetByTelegramID(ctx, userID)
	if err != nil {
		return profileResult{}, err
	}
	state, d := profileView, session{profile: user}
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d}, nil
}

func (s *ProfileService) editFirstName(userID int64) profileResult {
	state, d, exists := fsm.Load[session](s.machine, userID)
	if !exists || state != profileView {
		return profileResult{status: profileStale}
	}
	state, d.field = profileFirstName, firstNameField
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d}
}
func (s *ProfileService) setFirstName(userID int64, text string) profileResult {
	state, d, exists := fsm.Load[session](s.machine, userID)
	if !exists || state != profileFirstName {
		return profileResult{status: profileStale}
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		return profileResult{state: state, session: d, status: profileInvalidName}
	}
	d.profile.FirstName, state = text, profilePreview
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d}
}

func (s *ProfileService) editLastName(userID int64) profileResult {
	state, d, exists := fsm.Load[session](s.machine, userID)
	if !exists || state != profileView {
		return profileResult{status: profileStale}
	}
	state, d.field = profileLastName, lastNameField
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d}
}
func (s *ProfileService) setLastName(userID int64, text string) profileResult {
	state, d, exists := fsm.Load[session](s.machine, userID)
	if !exists || state != profileLastName {
		return profileResult{status: profileStale}
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		return profileResult{state: state, session: d, status: profileInvalidName}
	}
	d.profile.LastName, state = text, profilePreview
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d}
}

func (s *ProfileService) editSurname(userID int64) profileResult {
	state, d, exists := fsm.Load[session](s.machine, userID)
	if !exists || state != profileView {
		return profileResult{status: profileStale}
	}
	state, d.field = profileSurname, surnameField
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d}
}
func (s *ProfileService) setSurname(userID int64, text string) profileResult {
	state, d, exists := fsm.Load[session](s.machine, userID)
	if !exists || state != profileSurname {
		return profileResult{status: profileStale}
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		return profileResult{state: state, session: d, status: profileInvalidName}
	}
	d.profile.Surname, state = text, profilePreview
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d}
}

func (s *ProfileService) clearSurname(userID int64) profileResult {
	state, d, exists := fsm.Load[session](s.machine, userID)
	if !exists || state != profileSurname {
		return profileResult{status: profileStale}
	}
	d.profile.Surname, state = "", profilePreview
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d}
}
func (s *ProfileService) confirm(ctx context.Context, userID int64) (profileResult, error) {
	state, d, exists := fsm.Load[session](s.machine, userID)
	if !exists || state != profilePreview {
		return profileResult{status: profileStale}, nil
	}
	var err error
	switch d.field {
	case firstNameField:
		err = s.repo.UpdateFirstName(ctx, userID, d.profile.FirstName)
	case lastNameField:
		err = s.repo.UpdateLastName(ctx, userID, d.profile.LastName)
	case surnameField:
		err = s.repo.UpdateSurname(ctx, userID, d.profile.Surname)
	default:
		return profileResult{}, errors.New("invalid profile field")
	}
	if err != nil {
		return profileResult{state: state, session: d}, fmt.Errorf("save profile: %w", err)
	}
	user, err := s.repo.GetByTelegramID(ctx, userID)
	if err != nil {
		return profileResult{state: state, session: d}, err
	}
	state, d = profileView, session{profile: user}
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, session: d, status: profileUpdated}, nil
}

func (s *ProfileService) cancelEdit(userID int64) profileResult {
	state, _, exists := fsm.Load[session](s.machine, userID)
	if !exists || state == profileView {
		return profileResult{status: profileStale}
	}
	s.machine.Clear(userID)
	return profileResult{status: profileCancelled}
}

func validName(text string) bool {
	return text != "" && utf8.RuneCountInString(text) <= 100 && !strings.HasPrefix(text, "/") && !strings.ContainsFunc(text, unicode.IsControl)
}
