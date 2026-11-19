package announcements

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
)

type Repository interface {
	AnnouncementRecipients(context.Context, int64, int64, int) ([]int64, error)
}

const (
	enterTextState fsm.StateID = "admin.announcements.text"
	previewState   fsm.StateID = "admin.announcements.preview"
)

type draft struct {
	text string
	keyboards.TokenizedDraft
}

type announcementStatus uint8

const (
	announcementMissing announcementStatus = iota + 1
	announcementCancelled
	announcementStale
	announcementTextRequired
	announcementTooLong
	broadcastInProgress
	broadcastStarted
)

type announcementResult struct {
	state  fsm.StateID
	draft  draft
	status announcementStatus
}

type delivery struct {
	authorID int64
	text     string
}

type report struct {
	sent   int64
	failed int64
	err    error
}

type Service struct {
	mu         sync.Mutex
	repo       Repository
	machine    *fsm.FSM
	active     map[int64]bool
	ctx        context.Context
	cancel     context.CancelFunc
	workers    sync.WaitGroup
	broadcasts chan struct{}
}

func NewService(ctx context.Context, machine *fsm.FSM, repo Repository) *Service {
	ctx, cancel := context.WithCancel(ctx)
	return &Service{
		repo:       repo,
		machine:    machine,
		active:     make(map[int64]bool),
		ctx:        ctx,
		cancel:     cancel,
		broadcasts: make(chan struct{}, 1),
	}
}

func (s *Service) enteringText(userID int64) bool {
	return s.machine.CurrentState(userID) == enterTextState
}

func (s *Service) begin(userID int64) announcementResult {
	s.mu.Lock()
	active := s.active[userID]
	s.mu.Unlock()
	if active {
		return announcementResult{status: broadcastInProgress}
	}
	d := draft{}
	d.SetScreenToken(rand.Text())
	s.machine.Replace(userID, enterTextState, d)
	return announcementResult{state: enterTextState, draft: d}
}

func (s *Service) setText(userID int64, text string) announcementResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists || state != enterTextState {
		return announcementResult{status: announcementMissing}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return announcementResult{state: state, draft: d, status: announcementTextRequired}
	}
	length := 0
	for _, r := range text {
		length += utf16.RuneLen(r)
	}
	if length > 4000 {
		return announcementResult{state: state, draft: d, status: announcementTooLong}
	}
	d.text, state = text, previewState
	d.SetScreenToken(rand.Text())
	s.machine.Replace(userID, state, d)
	return announcementResult{state: state, draft: d}
}

func (s *Service) edit(userID int64, token string) announcementResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists || state != previewState || !d.IsTokenValid(token) {
		return announcementResult{status: announcementStale}
	}
	state = enterTextState
	d.SetScreenToken(rand.Text())
	s.machine.Replace(userID, state, d)
	return announcementResult{state: state, draft: d}
}

func (s *Service) cancelDraft(userID int64, token string) announcementResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists || (state != enterTextState && state != previewState) || !d.IsTokenValid(token) {
		return announcementResult{status: announcementStale}
	}
	s.machine.Clear(userID)
	return announcementResult{status: announcementCancelled}
}

func (s *Service) prepareSend(userID int64, token string) (announcementResult, *delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return announcementResult{}, nil, errors.New("announcement service stopped")
	}
	if s.active[userID] {
		return announcementResult{status: broadcastInProgress}, nil, nil
	}
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists || state != previewState || !d.IsTokenValid(token) {
		return announcementResult{status: announcementStale}, nil, nil
	}
	s.machine.Clear(userID)
	s.active[userID] = true
	s.workers.Add(1)
	return announcementResult{status: broadcastStarted}, &delivery{authorID: userID, text: d.text}, nil
}

func (s *Service) send(job delivery, sendMessage func(context.Context, int64, string) error, sendReport func(context.Context, int64, report) error) {
	defer s.workers.Done()
	defer func() {
		s.mu.Lock()
		delete(s.active, job.authorID)
		s.mu.Unlock()
	}()

	select {
	case s.broadcasts <- struct{}{}:
	case <-s.ctx.Done():
		return
	}
	defer func() { <-s.broadcasts }()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	semaphore := make(chan struct{}, 4)
	var sent, failed atomic.Int64
	var result report
	defer func() {
		if s.ctx.Err() != nil {
			return
		}
		result.sent, result.failed = sent.Load(), failed.Load()
		if result.err != nil {
			slog.ErrorContext(s.ctx, "announcement broadcast stopped", "author_id", job.authorID, "error", result.err)
		}
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		defer cancel()
		if err := sendReport(ctx, job.authorID, result); err != nil {
			slog.ErrorContext(ctx, "send announcement report failed", "author_id", job.authorID, "error", err)
		}
	}()

	var afterID int64
	for {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		recipients, err := s.repo.AnnouncementRecipients(ctx, afterID, job.authorID, 100)
		cancel()
		if err != nil {
			result.err = err
			return
		}
		if len(recipients) == 0 {
			return
		}

		var batch sync.WaitGroup
		for _, userID := range recipients {
			if userID == job.authorID {
				continue
			}
			select {
			case <-ticker.C:
			case <-s.ctx.Done():
				result.err = s.ctx.Err()
			}
			if result.err != nil {
				break
			}
			select {
			case semaphore <- struct{}{}:
			case <-s.ctx.Done():
				result.err = s.ctx.Err()
			}
			if result.err != nil {
				break
			}

			batch.Add(1)
			go func() {
				defer batch.Done()
				defer func() { <-semaphore }()
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				defer cancel()
				if err := sendMessage(ctx, userID, job.text); err != nil {
					failed.Add(1)
					slog.ErrorContext(ctx, "announcement delivery failed", "recipient_id", userID, "error", err)
					return
				}
				sent.Add(1)
			}()
		}
		batch.Wait()
		if result.err != nil || len(recipients) < 100 {
			return
		}
		afterID = recipients[len(recipients)-1]
	}
}
