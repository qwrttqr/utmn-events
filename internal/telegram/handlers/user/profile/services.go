package profile

import (
	"context"
	"crypto/rand"
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

type session struct {
	profile domain.User
	state   fsm.StateID
	field   fsm.StateID
	token   string
}
type screen struct {
	text  string
	state fsm.StateID
	token string
}
type ProfileService struct {
	mu      sync.Mutex
	repo    Repository
	machine *fsm.FSM[string, session]
}

func NewService(repo Repository) *ProfileService {
	return &ProfileService{repo: repo, machine: fsm.NewInMemoryFSM[string, session]("idle")}
}
func (s *ProfileService) Cancel(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.machine.Finish(context.Background(), userID); err != nil {
		slog.Error("clear profile draft failed", "user_id", userID, "error", err)
	}
}
func (s *ProfileService) inState(userID int64, expected fsm.StateID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.machine.CurrentState(context.Background(), userID)
	if err != nil {
		slog.Error("load profile state failed", "error", err)
		return false
	}
	return state == expected
}
func (s *ProfileService) begin(ctx context.Context, userID int64) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, err := s.repo.GetByTelegramID(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	d := session{profile: user, state: profileView, token: rand.Text()}
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}

func (s *ProfileService) editFirstName(ctx context.Context, userID int64, callback string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != profileView || callback != "user:profile:"+d.token+":first_name" {
		return staleScreen(), nil
	}
	d.state, d.field, d.token = profileFirstName, profileFirstName, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}
func (s *ProfileService) setFirstName(ctx context.Context, userID int64, text string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != profileFirstName {
		return staleScreen(), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := render(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.FirstName, d.state, d.token = text, profilePreview, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}

func (s *ProfileService) editLastName(ctx context.Context, userID int64, callback string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != profileView || callback != "user:profile:"+d.token+":last_name" {
		return staleScreen(), nil
	}
	d.state, d.field, d.token = profileLastName, profileLastName, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}
func (s *ProfileService) setLastName(ctx context.Context, userID int64, text string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != profileLastName {
		return staleScreen(), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := render(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.LastName, d.state, d.token = text, profilePreview, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}

func (s *ProfileService) editSurname(ctx context.Context, userID int64, callback string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != profileView || callback != "user:profile:"+d.token+":surname" {
		return staleScreen(), nil
	}
	d.state, d.field, d.token = profileSurname, profileSurname, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}
func (s *ProfileService) setSurname(ctx context.Context, userID int64, text string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != profileSurname {
		return staleScreen(), nil
	}
	text = strings.TrimSpace(text)
	if !validName(text) {
		view := render(d)
		view.text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + view.text
		return view, nil
	}
	d.profile.Surname, d.state, d.token = text, profilePreview, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}

func (s *ProfileService) clearSurname(ctx context.Context, userID int64, callback string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != profileSurname || callback != "user:profile:"+d.token+":clear_surname" {
		return staleScreen(), nil
	}
	d.profile.Surname, d.state, d.token = "", profilePreview, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}
func (s *ProfileService) confirm(ctx context.Context, userID int64, callback string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != profilePreview || callback != "user:profile:"+d.token+":save" {
		return staleScreen(), nil
	}
	switch d.field {
	case profileFirstName:
		err = s.repo.UpdateFirstName(ctx, userID, d.profile.FirstName)
	case profileLastName:
		err = s.repo.UpdateLastName(ctx, userID, d.profile.LastName)
	case profileSurname:
		err = s.repo.UpdateSurname(ctx, userID, d.profile.Surname)
	default:
		return screen{}, errors.New("invalid profile field")
	}
	if err != nil {
		return render(d), fmt.Errorf("save profile: %w", err)
	}
	user, err := s.repo.GetByTelegramID(ctx, userID)
	if err != nil {
		return render(d), err
	}
	d = session{profile: user, state: profileView, token: rand.Text()}
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	view := render(d)
	view.text = "Профиль обновлён.\n\n" + view.text
	return view, nil
}
func (s *ProfileService) cancelEdit(ctx context.Context, userID int64, callback string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || callback != "user:profile:"+d.token+":cancel" {
		return staleScreen(), nil
	}
	user, err := s.repo.GetByTelegramID(ctx, userID)
	if err != nil {
		return render(d), err
	}
	d = session{profile: user, state: profileView, token: rand.Text()}
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return render(d), nil
}
func (s *ProfileService) load(ctx context.Context, userID int64) (session, bool, error) {
	state, err := s.machine.CurrentState(ctx, userID)
	if err != nil {
		return session{}, false, err
	}
	if state == "idle" {
		return session{}, false, nil
	}
	d, err := s.machine.GetData(ctx, userID, "session")
	if err != nil {
		return session{}, false, err
	}
	d.state = state
	return d, true, nil
}
func (s *ProfileService) save(ctx context.Context, userID int64, d session) error {
	if err := s.machine.SetData(ctx, userID, "session", d); err != nil {
		return err
	}
	return s.machine.Transition(ctx, userID, d.state)
}
func validName(text string) bool {
	return text != "" && utf8.RuneCountInString(text) <= 100 && !strings.HasPrefix(text, "/") && !strings.ContainsFunc(text, unicode.IsControl)
}
func render(d session) screen {
	surname := d.profile.Surname
	if surname == "" {
		surname = "не указано"
	}
	text := fmt.Sprintf("Мой профиль\n\nИмя: %s\nФамилия: %s\nОтчество: %s", d.profile.FirstName, d.profile.LastName, surname)
	switch d.state {
	case profileFirstName:
		text = "Введите новое имя."
	case profileLastName:
		text = "Введите новую фамилию."
	case profileSurname:
		text = "Введите новое отчество или нажмите «Убрать отчество»."
	case profilePreview:
		text = "Проверьте изменения:\n\n" + text
	}
	return screen{text: text, state: d.state, token: d.token}
}
func staleScreen() screen {
	return screen{text: "Эта кнопка устарела. Откройте профиль заново."}
}
