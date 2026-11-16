package registration

import (
	"context"
	"errors"
	"sync"

	"testing"
	"utmn-events-bot/internal/domain"
)

func TestRegistrationRequiresConfirmation(t *testing.T) {
	repo := newMemoryUsers()
	p := &testPipeline{Pipeline: New(repo), t: t}
	for _, text := range []string{"/admin", "Иван", "Иванов"} {
		if _, handled := p.process(1, "ivan", text, ""); !handled {
			t.Fatal("unregistered user reached normal handlers")
		}
	}
	p.process(1, "ivan", "", "registration:skip")
	if _, ok := repo.users[1]; ok {
		t.Fatal("registered before confirmation")
	}
	view, _ := p.process(1, "ivan", "", "admin:create_event")
	if !view.confirm {
		t.Fatal("unrelated callback must preserve confirmation step")
	}
	p.process(1, "ivan", "", "registration:confirm")
	profile := repo.users[1]
	if profile.FirstName != "Иван" || profile.LastName != "Иванов" || profile.Surname != "" || profile.Username != "ivan" {
		t.Fatal("unexpected saved profile")
	}
	if _, handled := p.process(1, "ivan", "/admin", ""); handled {
		t.Fatal("registered user was blocked")
	}
	if _, handled := p.process(1, "ivan", "", "registration:confirm"); !handled {
		t.Fatal("stale confirmation callback escaped registration flow")
	}
}

func TestValidationRestartAndCancel(t *testing.T) {
	repo := newMemoryUsers()
	p := &testPipeline{Pipeline: New(repo), t: t}
	p.process(1, "", "/start", "")
	for _, text := range []string{"", "/admin", "name\nname"} {
		p.process(1, "", text, "")
		if p.drafts[1].step != firstName {
			t.Fatal("invalid input advanced registration")
		}
	}
	p.process(1, "", "Иван", "")
	p.process(1, "", "", "registration:confirm")
	if _, ok := repo.users[1]; ok {
		t.Fatal("out-of-step confirmation registered user")
	}
	p.process(1, "", "", "registration:restart")
	if p.drafts[1].step != firstName || p.drafts[1].profile.FirstName != "" {
		t.Fatal("restart did not clear draft")
	}
	view, _ := p.process(1, "", "", "registration:cancel")
	if !view.start {
		t.Fatal("cancelled registration must offer a start button")
	}
	if _, ok := p.drafts[1]; ok {
		t.Fatal("cancel did not remove draft")
	}
	p.process(1, "", "", "registration:start")
	if p.drafts[1].step != firstName {
		t.Fatal("start button did not begin registration")
	}
}

func TestSkipButtonOnlyAdvancesSurname(t *testing.T) {
	repo := newMemoryUsers()
	p := &testPipeline{Pipeline: New(repo), t: t}
	p.process(1, "", "/start", "")
	p.process(1, "", "", "registration:skip")
	if p.drafts[1].step != firstName {
		t.Fatal("skip button bypassed required first name")
	}
	p.process(1, "", "Иван", "")
	p.process(1, "", "", "registration:skip")
	if p.drafts[1].step != lastName {
		t.Fatal("skip button bypassed required last name")
	}
	p.process(1, "", "Иванов", "")
	view, _ := p.process(1, "", "", "registration:skip")
	if !view.confirm || p.drafts[1].step != confirmation {
		t.Fatal("skip button did not advance to confirmation")
	}
	p.process(1, "", "", "registration:skip")
	if _, ok := repo.users[1]; ok {
		t.Fatal("repeated skip registered user without confirmation")
	}
}

func TestConcurrentUsersAndDuplicateConfirmation(t *testing.T) {
	repo := newMemoryUsers()
	p := &testPipeline{Pipeline: New(repo), t: t}
	var wg sync.WaitGroup
	for id := int64(1); id <= 20; id++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			for _, text := range []string{"/start", "Иван", "Иванов", "Иванович"} {
				p.process(id, "", text, "")
			}
			p.process(id, "", "", "registration:confirm")
			p.process(id, "", "", "registration:confirm")
		}(id)
	}
	wg.Wait()
	if len(repo.users) != 20 || len(p.drafts) != 0 {
		t.Fatal("unexpected number of profiles or remaining drafts")
	}
}

// The fake is shared between pipeline instances to simulate durable storage.
type memoryUsers struct {
	mu       sync.Mutex
	users    map[int64]domain.User
	readErr  error
	writeErr error
}

func newMemoryUsers() *memoryUsers { return &memoryUsers{users: make(map[int64]domain.User)} }

func (r *memoryUsers) IsRegistered(ctx context.Context, id int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return false, r.readErr
	}
	_, exists := r.users[id]
	return exists, nil
}

func (r *memoryUsers) Create(ctx context.Context, user domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeErr != nil {
		return r.writeErr
	}
	if _, exists := r.users[user.TelegramID]; !exists {
		r.users[user.TelegramID] = user
	}
	return nil
}

type testPipeline struct {
	*Pipeline
	t *testing.T
}

func (p *testPipeline) process(id int64, username, text, callback string) (screen, bool) {
	p.t.Helper()
	view, handled, err := p.Pipeline.process(context.Background(), id, username, text, callback)
	if err != nil {
		p.t.Fatal(err)
	}
	return view, handled
}

func TestDatabaseReadFailureBlocksHandlers(t *testing.T) {
	repo := newMemoryUsers()
	repo.readErr = errors.New("database unavailable")
	p := New(repo)
	_, handled, err := p.process(context.Background(), 1, "", "/admin", "")
	if !handled || !errors.Is(err, repo.readErr) {
		t.Fatal("database error must block handler and preserve cause")
	}
	if len(p.drafts) != 0 {
		t.Fatal("database failure must not start onboarding")
	}
}

func TestSaveFailurePreservesDraftForRetry(t *testing.T) {
	repo := newMemoryUsers()
	p := &testPipeline{Pipeline: New(repo), t: t}
	for _, text := range []string{"/start", "Иван", "Иванов", "Иванович"} {
		p.process(1, "", text, "")
	}
	repo.writeErr = errors.New("database unavailable")
	view, handled, err := p.Pipeline.process(context.Background(), 1, "", "", "registration:confirm")
	if !handled || !view.confirm || !errors.Is(err, repo.writeErr) {
		t.Fatal("save failure must preserve confirmation and error cause")
	}
	if p.drafts[1].step != confirmation || len(repo.users) != 0 {
		t.Fatal("save failure lost draft or registered user")
	}
	repo.writeErr = nil
	p.process(1, "", "", "registration:confirm")
	if len(repo.users) != 1 || len(p.drafts) != 0 {
		t.Fatal("retry did not complete registration")
	}

	// A new flow instance checks the repository rather than its own memory.
	restarted := New(repo)
	_, handled, err = restarted.process(context.Background(), 1, "", "/admin", "")
	if err != nil || handled {
		t.Fatal("registered user was not recognized after pipeline restart")
	}
}
