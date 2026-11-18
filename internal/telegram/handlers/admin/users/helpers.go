package users

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
		if isAdmin(user, adminIDs) {
			role = "admin"
		}
		lines = append(lines, fmt.Sprintf("%d. %s\nРоль: %s", offset+i+1, userDetails(user), role))
	}
	if len(lines) == 0 {
		return "Список пуст."
	}
	return strings.Join(lines, "\n\n")
}

func isAdmin(user domain.User, adminIDs []int64) bool {
	return user.Role == "admin" || slices.Contains(adminIDs, user.TelegramID)
}

func userDetails(user domain.User) string {
	name := strings.TrimSpace(user.LastName + " " + user.FirstName + " " + user.Surname)
	if user.Username != "" {
		name += " (@" + user.Username + ")"
	}
	return fmt.Sprintf("%s\nTelegram ID: %d", name, user.TelegramID)
}
