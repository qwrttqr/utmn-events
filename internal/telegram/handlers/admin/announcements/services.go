package announcements

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"utmn-events-bot/internal/fsm"
)

type Repository interface {
	AnnouncementRecipients(context.Context, int64, int64, int) ([]int64, error)
}

const (
	textState    fsm.StateID = "admin.announcements.text"
	previewState fsm.StateID = "admin.announcements.preview"
)

type draft struct {
	token string
	text  string
	state fsm.StateID
}

type screen struct {
	text     string
	token    string
	state    fsm.StateID
	finished bool
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

type AnnouncementsService struct {
	mu         sync.Mutex
	repo       Repository
	machine    *fsm.FSM[string, draft]
	active     map[int64]bool
	closed     bool
	ctx        context.Context
	cancel     context.CancelFunc
	workers    sync.WaitGroup
	broadcasts chan struct{}
}

func NewService(ctx context.Context, repo Repository) *AnnouncementsService {
	ctx, cancel := context.WithCancel(ctx)
	return &AnnouncementsService{
		repo:       repo,
		machine:    fsm.NewInMemoryFSM[string, draft]("idle"),
		active:     make(map[int64]bool),
		ctx:        ctx,
		cancel:     cancel,
		broadcasts: make(chan struct{}, 1),
	}
}

func (s *AnnouncementsService) Close() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.workers.Wait()
}

func (s *AnnouncementsService) Cancel(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.machine.Finish(context.Background(), userID); err != nil {
		slog.Error("clear announcement draft failed", "user_id", userID, "error", err)
	}
}

func (s *AnnouncementsService) enteringText(userID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.machine.CurrentState(context.Background(), userID)
	if err != nil {
		slog.Error("read announcement state failed", "error", err)
		return false
	}
	return state == textState
}

func (s *AnnouncementsService) begin(ctx context.Context, userID int64) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[userID] {
		return screen{text: "Ваше объявление уже рассылается. Дождитесь отчёта.", finished: true}, nil
	}
	d := draft{token: rand.Text(), state: textState}
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return draftScreen(d), nil
}

func (s *AnnouncementsService) setText(ctx context.Context, userID int64, text string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || d.state != textState {
		return screen{text: "Откройте создание объявления заново.", finished: true}, nil
	}
	text = strings.TrimSpace(text)
	if text == "" || strings.HasPrefix(text, "/") {
		view := draftScreen(d)
		view.text = "Отправьте текст объявления.\n\n" + view.text
		return view, nil
	}
	length := 0
	for _, r := range text {
		length += utf16.RuneLen(r)
	}
	if length > 4000 {
		view := draftScreen(d)
		view.text = "Текст слишком длинный: максимум 4000 символов, эмодзи могут занимать два символа.\n\n" + view.text
		return view, nil
	}
	d.text, d.state, d.token = text, previewState, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return draftScreen(d), nil
}

func (s *AnnouncementsService) edit(ctx context.Context, userID int64, callback string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || !validCallback(d, callback, "edit") || d.state != previewState {
		return staleScreen(), nil
	}
	d.state, d.token = textState, rand.Text()
	if err := s.save(ctx, userID, d); err != nil {
		return screen{}, err
	}
	return draftScreen(d), nil
}

func (s *AnnouncementsService) cancelDraft(ctx context.Context, userID int64, callback string) (screen, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, err
	}
	if !exists || !validCallback(d, callback, "cancel") {
		return staleScreen(), nil
	}
	if err := s.machine.Finish(ctx, userID); err != nil {
		return screen{}, err
	}
	return screen{text: "Создание объявления отменено.", finished: true}, nil
}

func (s *AnnouncementsService) prepareSend(ctx context.Context, userID int64, callback string) (screen, *delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return screen{}, nil, errors.New("announcement service stopped")
	}
	if s.active[userID] {
		return screen{text: "Ваше объявление уже рассылается. Дождитесь отчёта.", finished: true}, nil, nil
	}
	d, exists, err := s.load(ctx, userID)
	if err != nil {
		return screen{}, nil, err
	}
	if !exists || d.state != previewState || !validCallback(d, callback, "send") {
		return staleScreen(), nil, nil
	}
	if err := s.machine.Finish(ctx, userID); err != nil {
		return screen{}, nil, err
	}
	s.active[userID] = true
	s.workers.Add(1)
	return screen{text: "Рассылка началась. После завершения придёт отчёт.", finished: true}, &delivery{authorID: userID, text: "Объявление\n\n" + d.text}, nil
}

func (s *AnnouncementsService) send(job delivery, sendMessage func(context.Context, int64, string) error) {
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
		if err := sendMessage(ctx, job.authorID, reportText(result)); err != nil {
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

func (s *AnnouncementsService) load(ctx context.Context, userID int64) (draft, bool, error) {
	state, err := s.machine.CurrentState(ctx, userID)
	if err != nil {
		return draft{}, false, err
	}
	if state != textState && state != previewState {
		return draft{}, false, nil
	}
	d, err := s.machine.GetData(ctx, userID, "draft")
	if err != nil {
		return draft{}, false, err
	}
	d.state = state
	return d, true, nil
}

func (s *AnnouncementsService) save(ctx context.Context, userID int64, d draft) error {
	if err := s.machine.SetData(ctx, userID, "draft", d); err != nil {
		return err
	}
	return s.machine.Transition(ctx, userID, d.state)
}

func validCallback(d draft, callback, action string) bool {
	return callback == "admin:announcement:"+d.token+":"+action
}

func draftScreen(d draft) screen {
	text := "Создание объявления."
	if d.state == previewState {
		text = "Проверьте объявление:\n\n" + d.text + "\n\nОтправить?"
	}
	return screen{text: text, token: d.token, state: d.state}
}

func staleScreen() screen {
	return screen{text: "Эта кнопка устарела. Откройте создание объявления заново.", finished: true}
}

func reportText(result report) string {
	text := fmt.Sprintf("Рассылка завершена.\nДоставлено: %d\nНе удалось доставить: %d", result.sent, result.failed)
	if result.err != nil {
		text = fmt.Sprintf("Рассылка прервана.\nДоставлено: %d\nНе удалось доставить: %d\nОстальные пользователи не получили объявление.", result.sent, result.failed)
	}
	return text
}
