package tags

import (
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"slices"
	"time"

	"utmn-events-bot/internal/telegram/navigation"
)

func cloneSession(s session) session {
	s.selected = maps.Clone(s.selected)
	s.tags = slices.Clone(s.tags)
	s.events = slices.Clone(s.events)
	return s
}

func (f *Flow) page(userID int64, view screen, snapshot *session) navigation.Page {
	page := navigation.Page{Key: "admin:tags:result", Text: view.text, Keyboard: keyboard(view), Admin: true, ClearHistory: view.completed || snapshot == nil}
	if snapshot == nil {
		return page
	}
	saved := cloneSession(*snapshot)
	page.Key = fmt.Sprintf("admin:tags:%d:%d", saved.mode, saved.event.ID)
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		s := cloneSession(saved)
		s.token = rand.Text()
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		view, err := f.render(dbCtx, &s)
		if err != nil {
			return navigation.Page{}, err
		}
		f.sessions[userID] = s
		return f.page(userID, view, &s), nil
	}
	return page
}
