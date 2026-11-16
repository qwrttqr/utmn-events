package events

import (
	"context"
	"fmt"
	"slices"

	"utmn-events-bot/internal/telegram/navigation"
)

func cloneDraft(d eventDraft) eventDraft {
	d.event.Tags = slices.Clone(d.event.Tags)
	if d.event.Capacity != nil {
		capacity := *d.event.Capacity
		d.event.Capacity = &capacity
	}
	return d
}

func (f *Flow) page(userID int64, view eventScreen, snapshot *eventDraft) navigation.Page {
	page := navigation.Page{Key: "admin:event:result", Text: view.text, PhotoFileID: view.photoID, Keyboard: eventKeyboard(view), Admin: true, ClearHistory: snapshot == nil}
	if snapshot == nil {
		return page
	}
	saved := cloneDraft(*snapshot)
	page.Key = fmt.Sprintf("admin:event:%s:%d", saved.token, saved.step)
	page.Restore = func(context.Context) (navigation.Page, error) {
		f.mu.Lock()
		d := cloneDraft(saved)
		f.drafts[userID] = d
		view := f.prompt(d)
		f.mu.Unlock()
		return f.page(userID, view, &d), nil
	}
	return page
}
