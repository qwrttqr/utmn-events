package main

import (
	"context"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type updateDispatcher struct {
	router  *bot.Bot
	slots   chan struct{}
	workers sync.WaitGroup
	mu      sync.Mutex
	tails   map[int64]chan struct{}
}

func newUpdateDispatcher(router *bot.Bot) *updateDispatcher {
	return &updateDispatcher{
		router: router,
		slots:  make(chan struct{}, 128),
		tails:  make(map[int64]chan struct{}),
	}
}

func (d *updateDispatcher) enqueue(ctx context.Context, _ *bot.Bot, update *models.Update) {
	var userID int64
	switch {
	case update.CallbackQuery != nil:
		userID = update.CallbackQuery.From.ID
	case update.Message != nil && update.Message.From != nil:
		userID = update.Message.From.ID
	default:
		return
	}

	select {
	case d.slots <- struct{}{}:
	case <-ctx.Done():
		return
	}

	done := make(chan struct{})
	d.mu.Lock()
	previous := d.tails[userID]
	d.tails[userID] = done
	d.mu.Unlock()

	d.workers.Go(func() {
		defer func() {
			d.mu.Lock()
			if d.tails[userID] == done {
				delete(d.tails, userID)
			}
			close(done)
			d.mu.Unlock()
			<-d.slots
		}()
		if previous != nil {
			<-previous
		}
		if ctx.Err() == nil {
			d.router.ProcessUpdate(ctx, update)
		}
	})
}
