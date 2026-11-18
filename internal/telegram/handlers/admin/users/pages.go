package users

import (
	"context"
	"fmt"
	"time"

	"utmn-events-bot/internal/telegram/navigation"
)

func expiredPage() navigation.Page {
	return navigation.Page{
		Key: "admin:users:expired", Text: "Эта кнопка устарела. Откройте список пользователей заново.", Admin: true,
		Keyboard: retryKeyboard("admin:users"),
	}
}

func (h *Handler) page(userID int64, result listResult) navigation.Page {
	if result.expired {
		return expiredPage()
	}
	text := "Все пользователи"
	if len(result.users) == 0 {
		text += "\n\nСписок пуст."
	}
	page := navigation.Page{
		Key:   "admin:users",
		Text:  text,
		Admin: true, Keyboard: usersKeyboard(result),
		Restore: func(ctx context.Context) (navigation.Page, error) {
			dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			restored, err := h.service.loadPage(dbCtx, userID, result.pageNumber)
			if err != nil {
				return navigation.Page{}, err
			}
			return h.page(userID, restored), nil
		}}
	return page
}

func (h *Handler) userPage(userID int64, selected userSelection) navigation.Page {
	role := "user"
	if isAdmin(selected.target, h.service.adminIDs) {
		role = "admin"
	}
	return navigation.Page{
		Key:      "admin:users:details",
		Text:     fmt.Sprintf("%s\nРоль: %s", userDetails(selected.target), role),
		Keyboard: userKeyboard(selected.target, selected.token, h.service.adminIDs),
		Admin:    true,
		Restore: func(ctx context.Context) (navigation.Page, error) {
			dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			restored, err := h.service.loadUser(dbCtx, userID, selected.target.TelegramID, selected.pageNumber)
			if err != nil {
				return navigation.Page{}, err
			}
			return h.userPage(userID, restored), nil
		},
	}
}

func promotionPage(draft userSelection, message string) navigation.Page {
	return navigation.Page{
		Key:          "admin:users:promotion",
		Text:         message + "Выдать права администратора этому пользователю?\n\n" + userDetails(draft.target),
		Keyboard:     promotionKeyboard(draft.token),
		Admin:        true,
		NoNavigation: true,
	}
}
