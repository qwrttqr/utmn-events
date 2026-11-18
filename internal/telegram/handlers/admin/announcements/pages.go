package announcements

import (
	"fmt"

	"utmn-events-bot/internal/fsm"
	adminmenu "utmn-events-bot/internal/telegram/handlers/admin/menu"
	"utmn-events-bot/internal/telegram/navigation"
)

func resultPage(result announcementResult) navigation.Page {
	switch result.status {
	case announcementCancelled:
		return adminmenu.GetMainPage()
	case announcementStale:
		return stalePage()
	case announcementMissing:
		return announcementPage("Откройте создание объявления заново.", fsm.Idle, true)
	case broadcastInProgress:
		return announcementPage("Ваше объявление уже рассылается. Дождитесь отчёта.", fsm.Idle, true)
	case broadcastStarted:
		return announcementPage("Рассылка началась. После завершения придёт отчёт.", fsm.Idle, true)
	case announcementTextRequired:
		page := draftPage(result.state, result.draft)
		page.Text = "Отправьте текст объявления.\n\n" + page.Text
		return page
	case announcementTooLong:
		page := draftPage(result.state, result.draft)
		page.Text = "Текст слишком длинный: максимум 4000 символов, эмодзи могут занимать два символа.\n\n" + page.Text
		return page
	}
	return draftPage(result.state, result.draft)
}

func announcementPage(text string, state fsm.StateID, finished bool) navigation.Page {
	return navigation.Page{
		Key:          "admin:announcement",
		Text:         text,
		Keyboard:     announcementKeyboard(state),
		Admin:        true,
		NoNavigation: true,
		ClearHistory: finished,
	}
}

func draftPage(state fsm.StateID, d draft) navigation.Page {
	text := "Создание объявления."
	if state == previewState {
		text = "Проверьте объявление:\n\n" + d.text + "\n\nОтправить?"
	}
	return announcementPage(text, state, false)
}

func stalePage() navigation.Page {
	return announcementPage("Эта кнопка устарела. Откройте создание объявления заново.", fsm.Idle, true)
}

func reportText(result report) string {
	text := fmt.Sprintf("Рассылка завершена.\nДоставлено: %d\nНе удалось доставить: %d", result.sent, result.failed)
	if result.err != nil {
		text = fmt.Sprintf("Рассылка прервана.\nДоставлено: %d\nНе удалось доставить: %d\nОстальные пользователи не получили объявление.", result.sent, result.failed)
	}
	return text
}
