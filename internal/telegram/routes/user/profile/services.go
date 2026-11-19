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
	"utmn-events-bot/internal/telegram/keyboards"
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

type draft struct {
	profile domain.User
	field   profileField
	keyboards.TokenizedDraft
}

type profileResult struct {
	state   fsm.StateID
	draft   draft
	message string
}

type ProfileService struct {
	repo    Repository
	machine *fsm.FSM
}

func NewService(machine *fsm.FSM, repo Repository) *ProfileService {
	return &ProfileService{repo: repo, machine: machine}
}

func (s *ProfileService) begin(ctx context.Context, userID int64) (profileResult, error) {
	user, err := s.repo.GetByTelegramID(ctx, userID)
	if err != nil {
		return profileResult{}, err
	}
	return s.replaceDraft(userID, profileView, draft{profile: user}), nil
}

func (s *ProfileService) edit(userID int64, d draft, state fsm.StateID, field profileField) profileResult {
	d.field = field
	return s.replaceDraft(userID, state, d)
}

func (s *ProfileService) setName(userID int64, text string, field profileField) profileResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists {
		return profileResult{state: fsm.Idle}
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		return profileResult{state: state, draft: d, message: "Введите от 1 до 100 символов без команд и переносов строк."}
	}
	switch field {
	case firstNameField:
		d.profile.FirstName = text
	case lastNameField:
		d.profile.LastName = text
	case surnameField:
		d.profile.Surname = text
	}
	return s.replaceDraft(userID, profilePreview, d)
}

func (s *ProfileService) clearSurname(userID int64, d draft) profileResult {
	d.profile.Surname = ""
	return s.replaceDraft(userID, profilePreview, d)
}

func (s *ProfileService) confirm(ctx context.Context, userID int64, d draft) (profileResult, error) {
	preview := profileResult{state: profilePreview, draft: d}
	var err error
	switch d.field {
	case firstNameField:
		err = s.repo.UpdateFirstName(ctx, userID, d.profile.FirstName)
	case lastNameField:
		err = s.repo.UpdateLastName(ctx, userID, d.profile.LastName)
	case surnameField:
		err = s.repo.UpdateSurname(ctx, userID, d.profile.Surname)
	default:
		return preview, errors.New("invalid profile field")
	}
	if err != nil {
		return preview, fmt.Errorf("save profile: %w", err)
	}
	user, err := s.repo.GetByTelegramID(ctx, userID)
	if err != nil {
		return preview, err
	}
	result := s.replaceDraft(userID, profileView, draft{profile: user})
	result.message = "Профиль обновлён."
	return result, nil
}

func (s *ProfileService) cancelEdit(userID int64) {
	s.machine.Clear(userID)
}

func (s *ProfileService) replaceDraft(userID int64, state fsm.StateID, d draft) profileResult {
	d.RotateScreenToken()
	s.machine.Replace(userID, state, d)
	return profileResult{state: state, draft: d}
}

func validName(text string) bool {
	return text != "" && utf8.RuneCountInString(text) <= 100 && !strings.HasPrefix(text, "/") && !strings.ContainsFunc(text, unicode.IsControl)
}
