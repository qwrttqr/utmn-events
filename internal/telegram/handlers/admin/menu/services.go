package menu

import "utmn-events-bot/internal/telegram/navigation"

func Page() navigation.Page {
	return navigation.Page{Key: "admin:menu", Text: "Панель администратора\nВыберите действие:", Keyboard: menuKeyboard(), Admin: true, Root: true}
}
func unknownPage(key string) navigation.Page {
	return navigation.Page{Key: key, Text: "Неизвестное действие. Вернитесь в меню администратора.", Admin: true}
}
