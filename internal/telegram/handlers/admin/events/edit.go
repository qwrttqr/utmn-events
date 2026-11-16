package events

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/go-telegram/bot"

	"utmn-events-bot/internal/domain"
)

func (f *Flow) Edit(ctx context.Context, b *bot.Bot, userID, chatID int64, event domain.Event, field domain.EventField) error {
	steps := map[domain.EventField]eventStep{
		domain.EventTitle: eventTitle, domain.EventDescription: eventDescription,
		domain.EventPhoto: eventPhoto, domain.EventLocation: eventLocation,
		domain.EventDates: eventStart, domain.EventCapacity: eventCapacity, domain.EventTags: eventTags,
	}
	step, ok := steps[field]
	if !ok {
		return errors.New("unsupported event field")
	}
	if _, ok := f.repo.(UpdateRepository); !ok {
		return errors.New("event repository does not support updates")
	}
	d := cloneDraft(eventDraft{event: event, step: step, token: rand.Text(), updateField: field})
	f.mu.Lock()
	f.drafts[userID] = d
	view := f.prompt(d)
	f.mu.Unlock()
	f.show(ctx, b, userID, chatID, view)
	return nil
}
