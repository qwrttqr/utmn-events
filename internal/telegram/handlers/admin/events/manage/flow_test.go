package manage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
)

type repositoryMock struct {
	deletes           int
	deleted           bool
	deleteErr         error
	registrationCount int64
}

func (r *repositoryMock) List(context.Context, int, int) ([]domain.EventSummary, error) {
	if r.deleted {
		return nil, nil
	}
	return []domain.EventSummary{{ID: 1, Title: "Лекция"}}, nil
}
func (r *repositoryMock) Get(context.Context, int64) (domain.Event, error) {
	if r.deleted {
		return domain.Event{}, domain.ErrEventNotFound
	}
	return domain.Event{ID: 1, Title: "Лекция", RegistrationCount: r.registrationCount}, nil
}
func (r *repositoryMock) Delete(context.Context, int64) error {
	r.deletes++
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deleted = true
	return nil
}

type editorMock struct{}

func (editorMock) Edit(context.Context, *bot.Bot, int64, int64, domain.Event, domain.EventField) error {
	return nil
}

func action(t *testing.T, page navigation.Page, name string) string {
	t.Helper()
	for _, row := range page.Keyboard.InlineKeyboard {
		for _, button := range row {
			if len(button.CallbackData) > 64 {
				t.Fatal("callback too long")
			}
			if strings.HasSuffix(button.CallbackData, ":"+name) {
				return button.CallbackData
			}
		}
	}
	t.Fatalf("action %s missing", name)
	return ""
}

func click(t *testing.T, f *Flow, callback string) navigation.Page {
	t.Helper()
	page, edit, err := f.process(context.Background(), 1, callback)
	if err != nil || edit != nil {
		t.Fatalf("unexpected management result: %v", err)
	}
	return page
}

func TestDeletionRequiresCurrentConfirmationAndSupportsRetry(t *testing.T) {
	repo := &repositoryMock{registrationCount: 3}
	f := New(repo, editorMock{}, nil, nil)
	page := click(t, f, "admin:events")
	page = click(t, f, action(t, page, "open1"))
	page = click(t, f, action(t, page, "delete"))
	if repo.deletes != 0 || !strings.Contains(page.Text, "Лекция") || !strings.Contains(page.Text, "3") {
		t.Fatal("delete confirmation missing or deletion happened early")
	}
	stale := action(t, page, "confirm")
	page = click(t, f, action(t, page, "keep"))
	click(t, f, stale)
	if repo.deletes != 0 {
		t.Fatal("stale confirmation deleted event")
	}
	page = click(t, f, action(t, page, "delete"))
	confirm := action(t, page, "confirm")
	repo.deleteErr = errors.New("database unavailable")
	if _, _, err := f.process(context.Background(), 1, confirm); !errors.Is(err, repo.deleteErr) {
		t.Fatal("deletion error not returned")
	}
	if f.sessions[1].mode != confirmDelete {
		t.Fatal("failed deletion lost confirmation")
	}
	repo.deleteErr = nil
	page = click(t, f, confirm)
	if !page.ClearHistory || !repo.deleted {
		t.Fatal("deletion did not finish safely")
	}
	click(t, f, confirm)
	if repo.deletes != 2 {
		t.Fatal("repeated confirmation called Delete again")
	}
}

type usersMock struct {
	eventID int64
	offset  int
}

func (m *usersMock) ListForEvent(_ context.Context, eventID int64, limit, offset int) ([]domain.User, error) {
	m.eventID, m.offset = eventID, offset
	var users []domain.User
	for i := offset + 1; i <= 9 && len(users) < limit; i++ {
		users = append(users, domain.User{ID: int64(i), TelegramID: int64(100 + i), FirstName: "Участник", Role: "user"})
	}
	return users, nil
}

func TestEventParticipantsAreScopedAndPaginated(t *testing.T) {
	repo := &repositoryMock{registrationCount: 9}
	users := &usersMock{}
	f := New(repo, editorMock{}, users, []int64{101})
	page := click(t, f, "admin:events")
	page = click(t, f, action(t, page, "open1"))
	if !strings.Contains(page.Text, "Зарегистрировано: 9") {
		t.Fatal("event card is missing participant count")
	}
	page = click(t, f, action(t, page, "participants"))
	if users.eventID != 1 || users.offset != 0 || !page.Admin || !strings.Contains(page.Text, "Роль: admin") {
		t.Fatal("participants are not scoped to event or roles missing")
	}
	page = click(t, f, action(t, page, "page1"))
	if users.offset != 8 || !strings.Contains(page.Text, "Страница 2") {
		t.Fatal("participants next page failed")
	}
	f.Cancel(1)
	if _, err := page.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if users.offset != 8 || users.eventID != 1 {
		t.Fatal("participant page was not preserved")
	}
}

func TestEditingSelectsExistingEventAndNeverDeletes(t *testing.T) {
	repo := &repositoryMock{}
	f := New(repo, editorMock{}, nil, nil)
	page := click(t, f, "admin:events")
	page = click(t, f, action(t, page, "open1"))
	page = click(t, f, action(t, page, "edit"))
	_, edit, err := f.process(context.Background(), 1, action(t, page, "fieldtitle"))
	if err != nil || edit == nil || edit.event.ID != 1 || edit.field != domain.EventTitle || repo.deletes != 0 {
		t.Fatal("wrong event or field selected for edit")
	}
	if _, exists := f.sessions[1]; exists {
		t.Fatal("old management actions remain active during editing")
	}
	if _, err := page.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.sessions[1].mode != fields || repo.deletes != 0 {
		t.Fatal("Back did not restore field selection safely")
	}
}
