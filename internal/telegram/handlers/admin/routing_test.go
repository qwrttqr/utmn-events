package admin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/telegram/handlers/admin/events"
	"utmn-events-bot/internal/telegram/handlers/admin/events/manage"
	"utmn-events-bot/internal/telegram/handlers/admin/tags"
	adminusers "utmn-events-bot/internal/telegram/handlers/admin/users"
	"utmn-events-bot/internal/telegram/handlers/user"
	"utmn-events-bot/internal/telegram/navigation"
)

type telegramStub struct {
	requests []map[string]string
}

func (s *telegramStub) Do(request *http.Request) (*http.Response, error) {
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		return nil, err
	}
	fields := map[string]string{}
	for key := range request.MultipartForm.Value {
		fields[key] = request.FormValue(key)
	}
	s.requests = append(s.requests, fields)
	result := `{"ok":true,"result":{"message_id":1,"chat":{"id":1,"type":"private"}}}`
	if strings.HasSuffix(request.URL.Path, "/answerCallbackQuery") {
		result = `{"ok":true,"result":true}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(result)), Header: make(http.Header)}, nil
}

func callbackUpdate(userID int64, data string) *models.Update {
	return &models.Update{CallbackQuery: &models.CallbackQuery{
		ID: "callback-id", From: models.User{ID: userID}, Data: data,
		Message: models.MaybeInaccessibleMessage{Message: &models.Message{
			ID: 1, Text: "Previous screen", Chat: models.Chat{ID: userID, Type: models.ChatTypePrivate},
		}},
	}}
}

func TestPanelSwitchingAndAdminAuthorization(t *testing.T) {
	f, err := events.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &telegramStub{}
	navigator := navigation.New([]int64{1}, func(id int64) navigation.Page { return user.DefaultPage(id, []int64{1}) })
	b, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithNotAsyncHandlers(), bot.WithHTTPClient(time.Second, client), bot.WithMiddlewares(navigator.Middleware))
	if err != nil {
		t.Fatal(err)
	}
	navigator.Register(b)
	cancel := Register(b, []int64{1}, f, tags.New(nil, nil), manage.New(nil, f, nil, []int64{1}), adminusers.New(nil, []int64{1}))
	navigator.SetCleanup(cancel)
	user.Register(b, []int64{1}, cancel)

	b.ProcessUpdate(context.Background(), callbackUpdate(1, "admin:create_event"))
	b.ProcessUpdate(context.Background(), callbackUpdate(1, "user:menu"))
	last := client.requests[len(client.requests)-1]
	if !strings.Contains(last["reply_markup"], "admin:menu") || f.HasDraft(1) {
		t.Fatal("admin user menu is missing switch button or retained event draft")
	}
	b.ProcessUpdate(context.Background(), callbackUpdate(1, "admin:menu"))
	last = client.requests[len(client.requests)-1]
	if !strings.Contains(last["reply_markup"], "user:menu") {
		t.Fatal("admin menu is missing switch back button")
	}
	b.ProcessUpdate(context.Background(), callbackUpdate(1, "home:menu"))
	last = client.requests[len(client.requests)-1]
	if !strings.Contains(last["reply_markup"], "user:menu") || !strings.Contains(last["text"], "Панель администратора") {
		t.Fatal("admin default landing is not the admin panel")
	}
	b.ProcessUpdate(context.Background(), callbackUpdate(2, "user:menu"))
	last = client.requests[len(client.requests)-1]
	if strings.Contains(last["reply_markup"], "admin:menu") {
		t.Fatal("regular user sees admin button")
	}
	for _, callback := range []string{"admin:menu", "admin:create_event", "admin:event:forged:8:save", "admin:tags", "admin:tag:forged:save", "admin:events", "admin:manage:forged:confirm", "admin:users", "admin:userlist:forged:page1"} {
		b.ProcessUpdate(context.Background(), callbackUpdate(2, callback))
		last = client.requests[len(client.requests)-1]
		if last["text"] != "Только для администратора." || f.HasDraft(2) {
			t.Fatal("non-admin callback was not rejected")
		}
	}
}
