package menu

import (
	"slices"

	adminmenu "utmn-events-bot/internal/telegram/handlers/admin/menu"
	"utmn-events-bot/internal/telegram/navigation"
)

func DefaultPage(userID int64, adminIDs []int64) navigation.Page {
	if slices.Contains(adminIDs, userID) {
		return adminmenu.GetMainPage()
	}
	return MenuPage(userID, adminIDs)
}
func MenuPage(userID int64, adminIDs []int64) navigation.Page {
	return navigation.Page{Key: "user:menu", Text: "Меню пользователя\nВыберите действие:", Root: true, Keyboard: menuKeyboard(slices.Contains(adminIDs, userID))}
}
