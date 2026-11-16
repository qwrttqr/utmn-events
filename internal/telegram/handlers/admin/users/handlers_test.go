package users

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
)

type repositoryMock struct{ offset int }

func (r *repositoryMock) List(_ context.Context, limit, offset int) ([]domain.User, error) {
	r.offset = offset
	var users []domain.User
	for i := offset + 1; i <= 10 && len(users) < limit; i++ {
		users = append(users, domain.User{ID: int64(i), TelegramID: int64(100 + i), FirstName: fmt.Sprintf("User%d", i), Role: "admin"})
	}
	return users, nil
}
func action(t *testing.T, page navigation.Page, name string) string {
	t.Helper()
	for _, row := range page.Keyboard.InlineKeyboard {
		for _, button := range row {
			if strings.HasSuffix(button.CallbackData, ":"+name) {
				return button.CallbackData
			}
		}
	}
	t.Fatalf("missing action %s", name)
	return ""
}

func TestUserListPaginationAndEffectiveRoles(t *testing.T) {
	repo := &repositoryMock{}
	f := New(repo, []int64{101})
	page, err := f.process(context.Background(), 101, "admin:users")
	if err != nil {
		t.Fatal(err)
	}
	if !page.Admin || strings.Count(page.Text, "Роль: admin") != 1 || !strings.Contains(page.Text, "Роль: user") {
		t.Fatal("user list roles do not match configured access")
	}
	page, err = f.process(context.Background(), 101, action(t, page, "page1"))
	if err != nil || repo.offset != 8 || !strings.Contains(page.Text, "User9") {
		t.Fatal("next page did not load correct users")
	}
	f.Cancel(101)
	if _, err := page.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.sessions[101].page != 1 || repo.offset != 8 {
		t.Fatal("navigation lost list pagination")
	}
}
