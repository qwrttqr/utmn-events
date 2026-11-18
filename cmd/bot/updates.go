package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	maxRunningUpdates = 128
	maxQueuedPerUser  = 8
)

type queuedUpdate struct {
	ctx    context.Context
	update *models.Update
}

type updateDispatcher struct {
	router  *bot.Bot
	slots   chan struct{}
	workers sync.WaitGroup
	mu      sync.Mutex
	queues  map[int64]chan queuedUpdate
}

func newUpdateDispatcher(router *bot.Bot) *updateDispatcher {
	return &updateDispatcher{
		router: router,
		slots:  make(chan struct{}, maxRunningUpdates),
		queues: make(map[int64]chan queuedUpdate),
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

	if ctx.Err() != nil {
		return
	}

	item := queuedUpdate{ctx: ctx, update: update}
	d.mu.Lock()
	queue := d.queues[userID]
	if queue == nil {
		d.queues[userID] = make(chan queuedUpdate, maxQueuedPerUser)
		d.mu.Unlock()
		d.workers.Go(func() { d.runUser(userID, item) })
		return
	}
	select {
	case queue <- item:
		d.mu.Unlock()
		return
	default:
	}
	d.mu.Unlock()

	if q := update.CallbackQuery; q != nil {
		d.workers.Go(func() {
			ackCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if _, err := d.router.AnswerCallbackQuery(ackCtx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: q.ID,
				Text:            "Слишком много запросов. Попробуйте ещё раз.",
			}); err != nil {
				slog.ErrorContext(ctx, "answer dropped callback failed", "update_id", update.ID, "error", err)
			}
		})
	}
}

func (d *updateDispatcher) runUser(userID int64, item queuedUpdate) {
	for {
		d.process(item)

		d.mu.Lock()
		select {
		case item = <-d.queues[userID]:
			d.mu.Unlock()
		default:
			delete(d.queues, userID)
			d.mu.Unlock()
			return
		}
	}
}

func (d *updateDispatcher) process(item queuedUpdate) {
	select {
	case <-item.ctx.Done():
		return
	case d.slots <- struct{}{}:
		defer func() { <-d.slots }()
	}
	if item.ctx.Err() == nil {
		d.router.ProcessUpdate(item.ctx, item.update)
	}
}
