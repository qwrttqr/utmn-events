package eventmanagement

import (
	"fmt"
	"slices"
	"strings"

	"utmn-events-bot/internal/domain"
)

func formatUsers(users []domain.User, offset int, adminIDs []int64) string {
	var lines []string
	for i, user := range users {
		role := "user"
		if slices.Contains(adminIDs, user.TelegramID) {
			role = "admin"
		}
		name := strings.TrimSpace(user.LastName + " " + user.FirstName + " " + user.Surname)
		username := ""
		if user.Username != "" {
			username = " (@" + user.Username + ")"
		}
		lines = append(lines, fmt.Sprintf("%d. %s%s\nTelegram ID: %d\nРоль: %s", offset+i+1, name, username, user.TelegramID, role))
	}
	if len(lines) == 0 {
		return "Список пуст."
	}
	return strings.Join(lines, "\n\n")
}
