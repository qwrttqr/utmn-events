package announcements

import (
	"fmt"

	"github.com/go-telegram/bot/models"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
)

func getPage(result announcementResult) navigation.Page {
	if result.state == fsm.Idle {
		return announcementPage(result.message, announcementKeyboard(fsm.Idle), true)
	}
	page := draftPage(result.state, result.draft)
	if result.message != "" {
		page.Text = result.message + "\n\n" + page.Text
	}
	return page
}

func broadcastInProgressPage() navigation.Page {
	return announcementPage("Ваше объявление уже рассылается. Дождитесь отчёта.", announcementKeyboard(fsm.Idle), true)
}

func announcementPage(
	text string,
	keyboard *models.InlineKeyboardMarkup,
	finished bool,
) navigation.Page {
	return navigation.Page{
		Key:          "admin:announcement",
		Text:         text,
		Keyboard:     keyboard,
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

	keyboard := keyboards.WithScreenToken(announcementKeyboard(state), d.Token)
	return announcementPage(text, keyboard, false)
}

func stalePage() navigation.Page {
	return announcementPage("Эта кнопка устарела. Откройте создание объявления заново.", announcementKeyboard(fsm.Idle), true)
}

func reportText(result report) string {
	text := fmt.Sprintf("Рассылка завершена.\nДоставлено: %d\nНе удалось доставить: %d", result.sent, result.failed)
	if result.err != nil {
		text = fmt.Sprintf("Рассылка прервана.\nДоставлено: %d\nНе удалось доставить: %d\nОстальные пользователи не получили объявление.", result.sent, result.failed)
	}
	return text
}
