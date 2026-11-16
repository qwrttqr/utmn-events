package events

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
)

type registrationsMock struct {
	enrollCalls, cancelCalls int
	actor                    int64
	enrolled                 map[int64]map[int64]bool
	err                      error
}

func (m *registrationsMock) IsEnrolled(_ context.Context, eventID, actor int64) (bool, error) {
	return m.enrolled[eventID][actor], nil
}
func (m *registrationsMock) Enroll(_ context.Context, eventID, actor int64) error {
	m.enrollCalls++
	m.actor = actor
	if m.err != nil {
		return m.err
	}
	if m.enrolled[eventID] == nil {
		m.enrolled[eventID] = make(map[int64]bool)
	}
	m.enrolled[eventID][actor] = true
	return nil
}
func (m *registrationsMock) Unenroll(_ context.Context, eventID, actor int64) error {
	m.cancelCalls++
	m.actor = actor
	delete(m.enrolled[eventID], actor)
	return nil
}

type repositoryMock struct {
	filters       []domain.EventFilter
	offsets       []int
	registrations *registrationsMock
}

func (m *repositoryMock) Browse(_ context.Context, filter domain.EventFilter, limit, offset int) ([]domain.EventSummary, error) {
	filter.TagIDs = slices.Clone(filter.TagIDs)
	m.filters = append(m.filters, filter)
	m.offsets = append(m.offsets, offset)
	var events []domain.EventSummary
	for i := offset + 1; i <= 10 && len(events) < limit; i++ {
		if filter.UserTelegramID != 0 && !m.registrations.enrolled[int64(i)][filter.UserTelegramID] {
			continue
		}
		events = append(events, domain.EventSummary{ID: int64(i), Title: fmt.Sprintf("Event %d", i)})
	}
	return events, nil
}
func (m *repositoryMock) Get(_ context.Context, id int64) (domain.Event, error) {
	capacity := int32(2)
	start := time.Date(2050, 1, 1, 0, 0, 0, 0, time.UTC)
	return domain.Event{ID: id, Title: "Лекция", StartsAt: start, EndsAt: start.Add(time.Hour), Capacity: &capacity, RegistrationCount: int64(len(m.registrations.enrolled[id]))}, nil
}

type tagsMock struct{}

func (tagsMock) List(_ context.Context, limit, offset int) ([]domain.Tag, error) {
	var tags []domain.Tag
	for i := offset + 1; i <= 10 && len(tags) < limit; i++ {
		tags = append(tags, domain.Tag{ID: int64(i), Name: fmt.Sprintf("tag%d", i)})
	}
	return tags, nil
}

func fixture(t *testing.T) (*Flow, *repositoryMock, *registrationsMock) {
	t.Helper()
	registrations := &registrationsMock{enrolled: make(map[int64]map[int64]bool)}
	repo := &repositoryMock{registrations: registrations}
	f, err := New(repo, tagsMock{}, registrations)
	if err != nil {
		t.Fatal(err)
	}
	f.now = func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
	return f, repo, registrations
}
func action(t *testing.T, page navigation.Page, name string) string {
	t.Helper()
	for _, row := range page.Keyboard.InlineKeyboard {
		for _, button := range row {
			if len(button.CallbackData) > 64 {
				t.Fatal("callback exceeds Telegram limit")
			}
			if strings.HasSuffix(button.CallbackData, ":"+name) {
				return button.CallbackData
			}
		}
	}
	t.Fatalf("missing action %s", name)
	return ""
}
func click(t *testing.T, f *Flow, callback string) navigation.Page {
	t.Helper()
	page, err := f.process(context.Background(), 111, callback)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestEnrollmentCancellationAndRefreshDoNotReplayWrites(t *testing.T) {
	f, repo, registrations := fixture(t)
	page := click(t, f, "user:events")
	page = click(t, f, action(t, page, "open1"))
	enroll := action(t, page, "enroll")
	page = click(t, f, enroll)
	if registrations.enrollCalls != 1 || registrations.actor != 111 || !registrations.enrolled[1][111] {
		t.Fatal("enrollment did not use the sender")
	}
	click(t, f, enroll)
	var err error
	page, err = page.Restore(context.Background())
	if err != nil || registrations.enrollCalls != 1 {
		t.Fatal("refresh replayed enrollment")
	}
	page = click(t, f, action(t, page, "cancel"))
	if registrations.cancelCalls != 0 {
		t.Fatal("cancellation occurred before confirmation")
	}
	confirm := action(t, page, "confirm")
	page = click(t, f, confirm)
	click(t, f, confirm)
	if registrations.cancelCalls != 1 || registrations.enrolled[1][111] || registrations.actor != 111 {
		t.Fatal("cancellation was repeated or affected the wrong user")
	}
	click(t, f, "user:registrations")
	filter := repo.filters[len(repo.filters)-1]
	if filter.UserTelegramID != 111 || filter.UpcomingOnly {
		t.Fatal("My registrations is not restricted to the actor or hides past registrations")
	}
}

func TestTagSearchRequiresSelectionAndPreservesItAcrossPages(t *testing.T) {
	f, repo, _ := fixture(t)
	page := click(t, f, "user:search")
	page = click(t, f, action(t, page, "search"))
	if len(repo.filters) != 0 || !strings.Contains(page.Text, "хотя бы один") {
		t.Fatal("empty selection performed a search")
	}
	page = click(t, f, action(t, page, "toggle1"))
	page = click(t, f, action(t, page, "page1"))
	page = click(t, f, action(t, page, "toggle9"))
	page = click(t, f, action(t, page, "search"))
	filter := repo.filters[len(repo.filters)-1]
	if !slices.Equal(filter.TagIDs, []int64{1, 9}) || !filter.UpcomingOnly || filter.UserTelegramID != 0 {
		t.Fatal("search lost selected tags or used wrong filters")
	}
	if !strings.Contains(page.Text, "tag1") || !strings.Contains(page.Text, "tag9") {
		t.Fatal("result does not explain selected tags")
	}
}

func TestBackRestoresListPageWithoutWrites(t *testing.T) {
	f, repo, registrations := fixture(t)
	page := click(t, f, "user:events")
	page = click(t, f, action(t, page, "page1"))
	listPage := page
	click(t, f, action(t, page, "open9"))
	f.Cancel(111)
	if _, err := listPage.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.sessions[111].page != 1 || repo.offsets[len(repo.offsets)-1] != 8 || registrations.enrollCalls != 0 {
		t.Fatal("Back did not restore pagination safely")
	}
}

func TestLastSeatFailureIsShownWithoutRegisteringUser(t *testing.T) {
	f, _, registrations := fixture(t)
	page := click(t, f, "user:events")
	page = click(t, f, action(t, page, "open1"))
	registrations.err = domain.ErrEventFull
	page = click(t, f, action(t, page, "enroll"))
	if !strings.Contains(page.Text, "Свободных мест больше нет") || registrations.enrolled[1][111] {
		t.Fatal("full event was not handled")
	}
}
