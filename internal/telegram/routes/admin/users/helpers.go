package users

import (
	"fmt"
	"slices"
	"strings"

	"utmn-events-bot/internal/domain"
)

func isAdmin(user domain.User, adminIDs []int64) bool {
	return user.Role == "admin" || slices.Contains(adminIDs, user.TelegramID)
}

func userLabel(user domain.User) string {
	name := strings.TrimSpace(user.LastName + " " + user.FirstName + " " + user.Surname)
	if user.Username != "" {
		name += " (@" + user.Username + ")"
	}
	if name == "" {
		return fmt.Sprintf("Пользователь %d", user.TelegramID)
	}
	return name
}

func userDetails(user domain.User) string {
	return fmt.Sprintf("%s\nTelegram ID: %d", userLabel(user), user.TelegramID)
}
