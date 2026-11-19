package announcements

import (
	"context"
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

var (
	errBroadcastInProgress = errors.New("announcement broadcast already in progress")
	errStaleAnnouncement   = errors.New("announcement draft is stale")
)

type announcementResult struct {
	state   fsm.StateID
	draft   draft
	message string
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

func (s *Service) begin(userID int64) (announcementResult, bool) {
	s.mu.Lock()
	active := s.active[userID]
	s.mu.Unlock()
	if active {
		return announcementResult{}, false
	}
	d := draft{}
	d.RotateScreenToken()
	s.machine.Replace(userID, enterTextState, d)
	return announcementResult{state: enterTextState, draft: d}, true
}

func (s *Service) setText(userID int64, text string) announcementResult {
	state, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists {
		return announcementResult{state: fsm.Idle, message: "Откройте создание объявления заново."}
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		return announcementResult{state: state, draft: d, message: "Отправьте текст объявления."}
	}
	length := 0
	for _, r := range text {
		length += utf16.RuneLen(r)
	}
	if length > 4000 {
		return announcementResult{state: state, draft: d, message: "Текст слишком длинный: максимум 4000 символов, эмодзи могут занимать два символа."}
	}
	d.text, state = text, previewState
	d.RotateScreenToken()
	s.machine.Replace(userID, state, d)
	return announcementResult{state: state, draft: d}
}

func (s *Service) edit(userID int64, token string) (announcementResult, bool) {
	_, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists || !d.IsTokenValid(token) {
		return announcementResult{}, false
	}
	d.RotateScreenToken()
	s.machine.Replace(userID, enterTextState, d)
	return announcementResult{state: enterTextState, draft: d}, true
}

func (s *Service) cancelDraft(userID int64, token string) bool {
	_, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists || !d.IsTokenValid(token) {
		return false
	}
	s.machine.Clear(userID)
	return true
}

func (s *Service) prepareSend(userID int64, token string) (delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return delivery{}, errors.New("announcement service stopped")
	}
	if s.active[userID] {
		return delivery{}, errBroadcastInProgress
	}
	_, d, exists := fsm.GetDraft[draft](s.machine, userID)
	if !exists || !d.IsTokenValid(token) {
		return delivery{}, errStaleAnnouncement
	}
	s.machine.Clear(userID)
	s.active[userID] = true
	s.workers.Add(1)
	return delivery{authorID: userID, text: d.text}, nil
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
