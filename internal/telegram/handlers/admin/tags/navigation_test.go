package tags

import (
	"context"
	"testing"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
)

type pagedTagRepository struct {
	Repository
	offset int
}

func (r *pagedTagRepository) List(_ context.Context, limit, offset int) ([]domain.Tag, error) {
	r.offset = offset
	return make([]domain.Tag, limit), nil
}

func TestRestoreTagEditorKeepsPageAndSelection(t *testing.T) {
	ctx := context.Background()
	const userID = 1
	machine := fsm.NewInMemoryFSM()
	repo := &pagedTagRepository{}
	service := NewService(machine, repo, nil)
	machine.Replace(userID, editTags, session{token: "current", selected: map[int64]bool{7: true}})

	view, err := service.changePage(ctx, userID, "admin:tag:current:page1")
	if err != nil {
		t.Fatal(err)
	}
	if repo.offset != pageSize {
		t.Fatalf("pagination queried offset %d, want %d", repo.offset, pageSize)
	}
	state, snapshot := service.snapshot(userID)
	page := (&Handler{service: service}).page(userID, view, state, snapshot)
	_, current, _ := fsm.Load[session](machine, userID)
	delete(current.selected, 7)
	machine.Clear(userID)

	restored, err := page.Restore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	state, saved, ok := fsm.Load[session](machine, userID)
	if !ok || state != editTags || saved.page != 1 || !saved.selected[7] || restored.Key != page.Key || repo.offset != pageSize {
		t.Fatalf("restored state=%q page=%d selection=%v key=%q offset=%d found=%v", state, saved.page, saved.selected, restored.Key, repo.offset, ok)
	}
}
