package users

import (
	"context"
	"fmt"
	"time"

	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/pagination"
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
	offset := result.pageNumber * pagination.Size
	page := navigation.Page{Key: "admin:users", Text: fmt.Sprintf("Все пользователи\n\n%s", formatUsers(result.users, offset, h.service.adminIDs)), Admin: true, Keyboard: usersKeyboard(result.pageNumber, result.more)}
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		restored, err := h.service.restore(dbCtx, userID, result.pageNumber)
		if err != nil {
			return navigation.Page{}, err
		}
		return h.page(userID, restored), nil
	}
	return page
}
