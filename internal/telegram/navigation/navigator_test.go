package navigation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type telegramMock struct {
	active                map[int]bool
	nextID                int
	sends, edits, deletes int
	failEdit              bool
}

func (m *telegramMock) Do(req *http.Request) (*http.Response, error) {
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		return nil, err
	}
	result := "true"
	switch {
	case strings.HasSuffix(req.URL.Path, "/sendMessage"), strings.HasSuffix(req.URL.Path, "/sendPhoto"):
		m.nextID++
		m.active[m.nextID] = true
		m.sends++
		result = fmt.Sprintf(`{"message_id":%d,"chat":{"id":1,"type":"private"}}`, m.nextID)
	case strings.HasSuffix(req.URL.Path, "/editMessageText"):
		if m.failEdit {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":false,"error_code":400,"description":"message to edit not found"}`))}, nil
		}
		m.edits++
		result = fmt.Sprintf(`{"message_id":%s,"chat":{"id":1,"type":"private"}}`, req.FormValue("message_id"))
	case strings.HasSuffix(req.URL.Path, "/deleteMessage"):
		id, _ := strconv.Atoi(req.FormValue("message_id"))
		delete(m.active, id)
		m.deletes++
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`))}, nil
}

func setup(t *testing.T) (*Navigator, *bot.Bot, *telegramMock) {
	t.Helper()
	n := New([]int64{1}, func(int64) Page { return Page{Key: "home", Text: "Home", Root: true} })
	m := &telegramMock{active: make(map[int]bool)}
	b, err := bot.New("test", bot.WithSkipGetMe(), bot.WithNotAsyncHandlers(), bot.WithHTTPClient(time.Second, m), bot.WithMiddlewares(n.Middleware))
	if err != nil {
		t.Fatal(err)
	}
	n.Register(b)
	return n, b, m
}

func show(t *testing.T, n *Navigator, b *bot.Bot, page Page) {
	t.Helper()
	ctx := context.WithValue(context.Background(), contextKey{}, request{navigator: n})
	if err := Show(ctx, b, 1, page); err != nil {
		t.Fatal(err)
	}
}

func navigate(n *Navigator, b *bot.Bot, action string) {
	s := n.sessions[1]
	b.ProcessUpdate(context.Background(), &models.Update{CallbackQuery: &models.CallbackQuery{
		ID: "query", From: models.User{ID: 1}, Data: "nav:" + s.token + ":" + action,
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: s.messages[len(s.messages)-1], Chat: models.Chat{ID: 1, Type: models.ChatTypePrivate}}},
	}})
}

func TestBackHomeAndRefreshRestoreWithoutRepeatingActions(t *testing.T) {
	n, b, api := setup(t)
	state := ""
	cleanups := 0
	n.SetCleanup(func(int64) { state = ""; cleanups++ })
	show(t, n, b, Page{Key: "admin", Text: "Admin", Admin: true, Root: true})
	step := Page{Key: "step1", Text: "First question", Admin: true}
	step.Restore = func(context.Context) (Page, error) { state = "step1"; return step, nil }
	show(t, n, b, step)
	show(t, n, b, Page{Key: "step2", Text: "Second question", Admin: true})
	navigate(n, b, "back")
	if state != "step1" || len(n.sessions[1].history) != 2 {
		t.Fatal("Back did not restore previous conversation")
	}
	navigate(n, b, "refresh")
	if state != "step1" || len(n.sessions[1].history) != 2 {
		t.Fatal("refresh changed history or lost state")
	}
	navigate(n, b, "home")
	if state != "" || len(n.sessions[1].history) != 1 || cleanups != 1 {
		t.Fatal("Home did not clear state and history")
	}
	if api.sends != 1 || api.edits != 5 {
		t.Fatal("text navigation did not reuse the same bot message")
	}
}

func TestRefreshReplacesHistoryAndCompletedActionCannotBeReopened(t *testing.T) {
	n, b, _ := setup(t)
	show(t, n, b, Page{Key: "home", Text: "Home", Root: true})
	for i := 0; i < 15; i++ {
		show(t, n, b, Page{Key: fmt.Sprint(i), Text: "Question"})
	}
	if len(n.sessions[1].history) != 10 || n.sessions[1].history[0].Key != "home" {
		t.Fatal("history limit lost Home")
	}
	show(t, n, b, Page{Key: "14", Text: "Updated question"})
	if len(n.sessions[1].history) != 10 {
		t.Fatal("same page duplicated history")
	}
	show(t, n, b, Page{Key: "saved", Text: "Saved", ClearHistory: true})
	if len(n.sessions[1].history) != 2 {
		t.Fatal("completed action left editable drafts in history")
	}
	navigate(n, b, "back")
	if n.sessions[1].history[0].Key != "home" || len(n.sessions[1].history) != 1 {
		t.Fatal("Back reopened completed action")
	}
}

func TestBackRestoresSeparateVisitsToSameStep(t *testing.T) {
	n, b, _ := setup(t)
	var restoredDraft string
	step := func(key, draft string) Page {
		page := Page{Key: key, Text: draft, History: HistoryPush}
		page.Restore = func(context.Context) (Page, error) {
			restoredDraft = draft
			return page, nil
		}
		return page
	}
	show(t, n, b, step("start", "initial draft"))
	show(t, n, b, step("preview", "draft ready to save"))
	show(t, n, b, step("start", "draft needs new dates"))

	retry := step("start", "draft needs new dates")
	retry.Text = "Invalid date. Try again."
	retry.History = HistoryMatchCurrent
	show(t, n, b, retry)
	if history := n.sessions[1].history; len(history) != 4 || history[3].Text != retry.Text {
		t.Fatalf("retry did not replace only the current visit: %+v", history)
	}
	navigate(n, b, "refresh")
	if restoredDraft != "draft needs new dates" || len(n.sessions[1].history) != 4 {
		t.Fatal("Refresh lost the current visit")
	}
	navigate(n, b, "back")
	if restoredDraft != "draft ready to save" {
		t.Fatalf("Back restored %q instead of the preview draft", restoredDraft)
	}
	navigate(n, b, "back")
	if restoredDraft != "initial draft" {
		t.Fatalf("Back restored %q instead of the initial draft", restoredDraft)
	}
}

func TestPhotoAndMultipartScreensAreRemovedOnHome(t *testing.T) {
	n, b, api := setup(t)
	show(t, n, b, Page{Key: "home", Text: "Home", Root: true})
	show(t, n, b, Page{Key: "preview", Text: strings.Repeat("я", 4500), PhotoFileID: "file-id"})
	if len(n.sessions[1].messages) != 4 || len(api.active) != 4 {
		t.Fatal("photo/multipart preview was not tracked")
	}
	navigate(n, b, "home")
	if len(api.active) != 1 || len(n.sessions[1].messages) != 1 {
		t.Fatal("Home left old preview messages")
	}
	api.failEdit = true
	show(t, n, b, Page{Key: "new", Text: "New"})
	if len(api.active) != 1 {
		t.Fatal("failed edit did not fall back to replacing the message")
	}
}

func TestUserInputCleanupAndStaleNavigation(t *testing.T) {
	n, b, api := setup(t)
	ctx := context.WithValue(context.Background(), contextKey{}, request{n, &models.Update{Message: &models.Message{ID: 999, Chat: models.Chat{ID: 1}}}})
	api.active[999] = true
	if err := Show(ctx, b, 1, Page{Key: "home", Text: "Home", Root: true}); err != nil {
		t.Fatal(err)
	}
	if api.active[999] {
		t.Fatal("processed user input was not deleted")
	}
	oldToken := n.sessions[1].token
	show(t, n, b, Page{Key: "child", Text: "Child"})
	b.ProcessUpdate(context.Background(), &models.Update{CallbackQuery: &models.CallbackQuery{
		ID: "old", From: models.User{ID: 1}, Data: "nav:" + oldToken + ":home",
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{ID: 1, Chat: models.Chat{ID: 1, Type: models.ChatTypePrivate}}},
	}})
	if len(n.sessions[1].history) != 2 {
		t.Fatal("stale Home button changed navigation")
	}
}
