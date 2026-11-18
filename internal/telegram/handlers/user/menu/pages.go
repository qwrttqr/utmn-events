package menu

import (
	adminmenu "utmn-events-bot/internal/telegram/handlers/admin/menu"
	"utmn-events-bot/internal/telegram/navigation"
)

func DefaultPage(isAdmin bool) navigation.Page {
	if isAdmin {
		return adminmenu.GetMainPage()
	}
	return GetMainPage(isAdmin)
}
func GetMainPage(isAdmin bool) navigation.Page {
	return navigation.Page{Key: "user:menu", Text: "Меню пользователя\nВыберите действие:", Root: true, Keyboard: menuKeyboard(isAdmin)}
}
