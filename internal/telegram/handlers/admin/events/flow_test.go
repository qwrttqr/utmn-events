package events

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"utmn-events-bot/internal/domain"
)

type eventRepositoryStub struct {
	created   []domain.Event
	err       error
	updated   []domain.Event
	fields    []domain.EventField
	updateErr error
}

func (r *eventRepositoryStub) Update(_ context.Context, event domain.Event, field domain.EventField) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updated = append(r.updated, event)
	r.fields = append(r.fields, field)
	return nil
}

func (r *eventRepositoryStub) Create(_ context.Context, event domain.Event) (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	r.created = append(r.created, event)
	return int64(len(r.created)), nil
}

func newTestFlow(t *testing.T) (*Flow, *eventRepositoryStub) {
	t.Helper()
	repo := &eventRepositoryStub{}
	f, err := New(repo)
	if err != nil {
		t.Fatal(err)
	}
	f.now = func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
	return f, repo
}

func eventAction(t *testing.T, view eventScreen, action string) string {
	t.Helper()
	for _, button := range view.buttons {
		if strings.HasSuffix(button.data, ":"+action) {
			if len(button.data) > 64 {
				t.Fatal("callback data exceeds Telegram limit")
			}
			return button.data
		}
	}
	t.Fatalf("action %s not found", action)
	return ""
}

func advanceEvent(t *testing.T, f *Flow, text, photo, callback string) eventScreen {
	t.Helper()
	view, err := f.process(context.Background(), 1, text, photo, callback)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func fillEvent(t *testing.T, f *Flow) eventScreen {
	t.Helper()
	f.begin(1)
	advanceEvent(t, f, "Лекция", "", "")
	advanceEvent(t, f, "Описание", "", "")
	advanceEvent(t, f, "", "telegram-file-id", "")
	advanceEvent(t, f, "Аудитория 101", "", "")
	advanceEvent(t, f, "25.12.2031 18:30", "", "")
	advanceEvent(t, f, "25.12.2031 20:00", "", "")
	advanceEvent(t, f, "25", "", "")
	return advanceEvent(t, f, "Go; лекция; GO", "", "")
}

func TestEventCreationSavesOnlyAfterConfirmation(t *testing.T) {
	f, repo := newTestFlow(t)
	view := fillEvent(t, f)
	if len(repo.created) != 0 || view.photoID != "telegram-file-id" {
		t.Fatal("preview saved the event early or lost the photo")
	}
	save := eventAction(t, view, "save")
	advanceEvent(t, f, "", "", save)
	advanceEvent(t, f, "", "", save)
	if len(repo.created) != 1 || f.HasDraft(1) {
		t.Fatal("duplicate confirmation created another event or left a draft")
	}
	event := repo.created[0]
	wantStart := time.Date(2031, 12, 25, 13, 30, 0, 0, time.UTC)
	if event.StartsAt.Location() != time.UTC || !event.StartsAt.Equal(wantStart) || !event.EndsAt.After(event.StartsAt) {
		t.Fatal("incorrect UTC conversion")
	}
	if event.Title != "Лекция" || event.Location != "Аудитория 101" || event.Capacity == nil || *event.Capacity != 25 {
		t.Fatal("event fields were not collected")
	}
	if strings.Join(event.Tags, ";") != "go;лекция" {
		t.Fatal("tags were not normalized and deduplicated")
	}
}

func TestEventSaveFailurePreservesPreview(t *testing.T) {
	f, repo := newTestFlow(t)
	view := fillEvent(t, f)
	save := eventAction(t, view, "save")
	repo.err = errors.New("database unavailable")
	view, err := f.process(context.Background(), 1, "", "", save)
	if !errors.Is(err, repo.err) || !f.HasDraft(1) || eventAction(t, view, "save") != save {
		t.Fatal("failed save did not preserve retry state and error cause")
	}
	repo.err = nil
	advanceEvent(t, f, "", "", save)
	if len(repo.created) != 1 {
		t.Fatal("retry did not save event")
	}
}

func TestEventValidationAndStaleButtons(t *testing.T) {
	f, repo := newTestFlow(t)
	old := f.begin(1)
	f.begin(1)
	advanceEvent(t, f, "", "", eventAction(t, old, "cancel"))
	if !f.HasDraft(1) {
		t.Fatal("old draft button cancelled current draft")
	}
	advanceEvent(t, f, "", "", "")
	if f.drafts[1].step != eventTitle {
		t.Fatal("empty title advanced wizard")
	}
	advanceEvent(t, f, "Лекция", "", "")
	description := f.prompt(f.drafts[1])
	photo := advanceEvent(t, f, "", "", eventAction(t, description, "skip"))
	advanceEvent(t, f, "", "", eventAction(t, description, "skip"))
	if f.drafts[1].step != eventPhoto {
		t.Fatal("repeated skip bypassed another step")
	}
	advanceEvent(t, f, "", "", eventAction(t, photo, "skip"))
	advanceEvent(t, f, "Адрес", "", "")
	for _, invalid := range []string{"yesterday", "01.01.2020 10:00"} {
		advanceEvent(t, f, invalid, "", "")
		if f.drafts[1].step != eventStart {
			t.Fatal("invalid start date advanced wizard")
		}
	}
	advanceEvent(t, f, "25.12.2031 18:30", "", "")
	advanceEvent(t, f, "25.12.2031 18:00", "", "")
	if f.drafts[1].step != eventEnd {
		t.Fatal("invalid end date advanced wizard")
	}
	advanceEvent(t, f, "25.12.2031 20:00", "", "")
	for _, invalid := range []string{"0", "-1", "2147483648"} {
		advanceEvent(t, f, invalid, "", "")
		if f.drafts[1].step != eventCapacity {
			t.Fatal("invalid capacity advanced wizard")
		}
	}
	view := advanceEvent(t, f, "", "", eventAction(t, f.prompt(f.drafts[1]), "skip"))
	view = advanceEvent(t, f, "", "", eventAction(t, view, "skip"))
	advanceEvent(t, f, "", "", eventAction(t, view, "save"))
	if len(repo.created) != 1 || repo.created[0].Capacity != nil || len(repo.created[0].Tags) != 0 {
		t.Fatal("optional fields were not skipped")
	}
}

func TestPanelSwitchDiscardsEventDraft(t *testing.T) {
	f, repo := newTestFlow(t)
	view := fillEvent(t, f)
	f.Cancel(1)
	advanceEvent(t, f, "", "", eventAction(t, view, "save"))
	if f.HasDraft(1) || len(repo.created) != 0 {
		t.Fatal("discarded event was saved")
	}
}

func TestExpiredPreviewCanBeCorrectedWithTextDates(t *testing.T) {
	f, repo := newTestFlow(t)
	view := fillEvent(t, f)
	original := f.drafts[1].event
	f.now = func() time.Time { return original.StartsAt.Add(time.Minute) }
	view = advanceEvent(t, f, "", "", eventAction(t, view, "save"))
	if f.drafts[1].step != eventStart || len(repo.created) != 0 {
		t.Fatal("expired start must return to date input without saving")
	}
	for _, input := range []string{"11.09.2005 22:00", "22:00", "31.02.2032 22:00"} {
		advanceEvent(t, f, input, "", "")
		if f.drafts[1].step != eventStart {
			t.Fatal("invalid text date advanced the wizard")
		}
	}
	advanceEvent(t, f, "26.12.2031 18:30", "", "")
	view = advanceEvent(t, f, "26.12.2031 20:00", "", "")
	if f.drafts[1].step != eventPreview || strings.Contains(view.text, "UTC+5") {
		t.Fatal("corrected text dates did not return to a clean preview")
	}
	advanceEvent(t, f, "", "", eventAction(t, view, "save"))
	if len(repo.created) != 1 || strings.Join(repo.created[0].Tags, ";") != strings.Join(original.Tags, ";") {
		t.Fatal("date correction lost event data")
	}
}

func TestNavigationRestoresDraftWithoutDatabaseWrites(t *testing.T) {
	f, repo := newTestFlow(t)
	view := fillEvent(t, f)
	d := cloneDraft(f.drafts[1])
	page := f.page(1, view, &d)
	f.Cancel(1)
	if _, err := page.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.drafts[1].step != eventPreview || f.drafts[1].event.Title != d.event.Title || len(repo.created) != 0 {
		t.Fatal("navigation lost draft or repeated event creation")
	}
}

func TestEditPastEventTitleDoesNotCreateAnotherEvent(t *testing.T) {
	f, repo := newTestFlow(t)
	f.drafts[1] = eventDraft{token: "edit", step: eventTitle, updateField: domain.EventTitle,
		event: domain.Event{ID: 42, Title: "Old", StartsAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}}
	view := advanceEvent(t, f, "New title", "", "")
	if f.drafts[1].step != eventPreview {
		t.Fatal("single field edit did not reach preview")
	}
	advanceEvent(t, f, "", "", eventAction(t, view, "save"))
	if len(repo.created) != 0 || len(repo.updated) != 1 || repo.updated[0].ID != 42 || repo.updated[0].Title != "New title" || repo.fields[0] != domain.EventTitle {
		t.Fatal("edit created another event or updated wrong field")
	}
}

func TestEditDatesCollectsBothDatesAndCapacityFailureKeepsDraft(t *testing.T) {
	f, repo := newTestFlow(t)
	f.drafts[1] = eventDraft{token: "edit", step: eventStart, updateField: domain.EventDates, event: domain.Event{ID: 42}}
	advanceEvent(t, f, "25.12.2031 18:30", "", "")
	if f.drafts[1].step != eventEnd {
		t.Fatal("dates edit must collect end date")
	}
	advanceEvent(t, f, "25.12.2031 18:00", "", "")
	if f.drafts[1].step != eventEnd {
		t.Fatal("invalid end accepted")
	}
	view := advanceEvent(t, f, "25.12.2031 20:00", "", "")
	advanceEvent(t, f, "", "", eventAction(t, view, "save"))
	if len(repo.updated) != 1 || repo.fields[0] != domain.EventDates || !repo.updated[0].EndsAt.After(repo.updated[0].StartsAt) {
		t.Fatal("dates were not updated together")
	}
	f.drafts[1] = eventDraft{token: "capacity", step: eventCapacity, updateField: domain.EventCapacity, event: domain.Event{ID: 42}}
	view = advanceEvent(t, f, "1", "", "")
	repo.updateErr = domain.ErrCapacityBelowRegistrations
	view = advanceEvent(t, f, "", "", eventAction(t, view, "save"))
	if !f.HasDraft(1) || !strings.Contains(view.text, "зарегистрированных") {
		t.Fatal("capacity rejection lost draft or user explanation")
	}
}
