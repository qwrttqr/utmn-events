package tags

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"utmn-events-bot/internal/domain"
)

type repositoryMock struct {
	create func(string) (domain.Tag, error)
	set    func(int64, []int64) error
}

func (m *repositoryMock) Create(_ context.Context, name string) (domain.Tag, error) {
	return m.create(name)
}
func (m *repositoryMock) List(_ context.Context, limit, offset int) ([]domain.Tag, error) {
	var result []domain.Tag
	for i := offset + 1; i <= 10 && len(result) < limit; i++ {
		result = append(result, domain.Tag{ID: int64(i), Name: fmt.Sprintf("tag-%02d", i)})
	}
	return result, nil
}
func (m *repositoryMock) ForEvent(_ context.Context, _ int64) ([]domain.Tag, error) {
	return []domain.Tag{{ID: 2, Name: "tag-02"}, {ID: 9, Name: "tag-09"}}, nil
}
func (m *repositoryMock) SetForEvent(_ context.Context, id int64, ids []int64) error {
	return m.set(id, ids)
}

type eventsMock struct{}

func (eventsMock) List(context.Context, int, int) ([]domain.EventSummary, error) {
	return []domain.EventSummary{{ID: 42, Title: "Лекция"}}, nil
}

func action(t *testing.T, view screen, suffix string) string {
	t.Helper()
	for _, button := range view.buttons {
		if len(button.data) > 64 {
			t.Fatal("callback exceeds Telegram limit")
		}
		if strings.HasSuffix(button.data, ":"+suffix) {
			return button.data
		}
	}
	t.Fatalf("action %s not found", suffix)
	return ""
}

func click(t *testing.T, f *Flow, view screen, name string) screen {
	t.Helper()
	next, err := f.process(context.Background(), 1, "", action(t, view, name))
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func beginAssignment(t *testing.T, f *Flow) screen {
	t.Helper()
	view, err := f.begin(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	view = click(t, f, view, "assign")
	return click(t, f, view, "event42")
}

func TestCreateTagNormalizesAndValidatesName(t *testing.T) {
	calls := 0
	repo := &repositoryMock{create: func(name string) (domain.Tag, error) {
		calls++
		if name != "go" {
			t.Fatalf("unexpected normalized name %q", name)
		}
		return domain.Tag{ID: 1, Name: name}, nil
	}}
	f := New(repo, eventsMock{})
	view, _ := f.begin(context.Background(), 1)
	click(t, f, view, "create")
	for _, name := range []string{"", "go;sport", "/admin", "two\nlines"} {
		if _, err := f.process(context.Background(), 1, name, ""); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("invalid name reached repository")
	}
	view, err := f.process(context.Background(), 1, " GO ", "")
	if err != nil || calls != 1 || !strings.Contains(view.text, "go") {
		t.Fatal("valid tag was not created")
	}
}

func TestSelectionAcrossPagesAndSaveRetry(t *testing.T) {
	writes := 0
	failed := true
	repo := &repositoryMock{set: func(eventID int64, ids []int64) error {
		writes++
		slices.Sort(ids)
		if eventID != 42 || !slices.Equal(ids, []int64{1, 2}) {
			t.Fatalf("unexpected selection %d %v", eventID, ids)
		}
		if failed {
			return errors.New("database unavailable")
		}
		return nil
	}}
	f := New(repo, eventsMock{})
	view := beginAssignment(t, f)
	old := action(t, view, "save")
	view = click(t, f, view, "toggle1")
	view = click(t, f, view, "page1")
	view = click(t, f, view, "toggle9")
	if writes != 0 {
		t.Fatal("selection was saved before Save")
	}
	if _, err := f.process(context.Background(), 1, "", old); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatal("stale Save button modified assignments")
	}
	view, err := f.process(context.Background(), 1, "", action(t, view, "save"))
	if err == nil || !f.sessions[1].selected[1] || f.sessions[1].selected[9] {
		t.Fatal("save failure lost selection")
	}
	failed = false
	view = click(t, f, view, "save")
	if writes != 2 || !strings.Contains(view.text, "сохранены") {
		t.Fatal("retry did not save selection")
	}
	f.process(context.Background(), 1, "", old)
	if writes != 2 {
		t.Fatal("old Save repeated after completion")
	}
}

func TestClearAndCancelAssignments(t *testing.T) {
	writes := 0
	repo := &repositoryMock{set: func(_ int64, ids []int64) error {
		writes++
		if len(ids) != 0 {
			t.Fatal("clear did not remove all selected tags")
		}
		return nil
	}}
	f := New(repo, eventsMock{})
	view := beginAssignment(t, f)
	view = click(t, f, view, "clear")
	view = click(t, f, view, "save")
	if writes != 1 {
		t.Fatal("empty assignment was not saved")
	}
	view = beginAssignment(t, f)
	save := action(t, view, "save")
	f.Cancel(1)
	f.process(context.Background(), 1, "", save)
	if writes != 1 {
		t.Fatal("cancelled selection was saved")
	}
}

func TestNavigationRestoresTagSelectionWithoutSaving(t *testing.T) {
	writes := 0
	repo := &repositoryMock{set: func(int64, []int64) error { writes++; return nil }}
	f := New(repo, eventsMock{})
	view := beginAssignment(t, f)
	view = click(t, f, view, "toggle1")
	snapshot := cloneSession(f.sessions[1])
	page := f.page(1, view, &snapshot)
	f.Cancel(1)
	if _, err := page.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if writes != 0 || !f.sessions[1].selected[1] || !f.sessions[1].selected[9] || f.sessions[1].event.ID != 42 {
		t.Fatal("navigation lost selection or saved tags during restore")
	}
}
