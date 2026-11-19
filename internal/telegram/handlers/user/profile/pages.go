package profile

import (
	"context"
	"fmt"

	"utmn-events-bot/internal/fsm"
	usermenu "utmn-events-bot/internal/telegram/handlers/user/menu"
	"utmn-events-bot/internal/telegram/navigation"
)

func (h *Handler) getPage(ctx context.Context, userID int64, result profileResult) navigation.Page {
	var message string
	switch result.status {
	case profileStale:
		return stalePage()
	case profileCancelled:
		return usermenu.GetMainPage(h.isAdmin(ctx, userID))
	case profileInvalidName:
		message = "Введите от 1 до 100 символов без команд и переносов строк."
	case profileUpdated:
		message = "Профиль обновлён."
	}
	if result.state == "" {
		return navigation.Page{}
	}
	page := render(result.state, result.session)
	if message != "" {
		page.Text = message + "\n\n" + page.Text
	}
	return page
}

func render(state fsm.StateID, d session) navigation.Page {
	surname := d.profile.Surname
	if surname == "" {
		surname = "не указано"
	}
	text := fmt.Sprintf("Мой профиль\n\nИмя: %s\nФамилия: %s\nОтчество: %s", d.profile.FirstName, d.profile.LastName, surname)
	switch state {
	case profileFirstName:
		text = "Введите новое имя."
	case profileLastName:
		text = "Введите новую фамилию."
	case profileSurname:
		text = "Введите новое отчество или нажмите «Убрать отчество»."
	case profilePreview:
		text = "Проверьте изменения:\n\n" + text
	}
	return profilePage(text, state)
}

func profilePage(text string, state fsm.StateID) navigation.Page {
	return navigation.Page{
		Key:          "user:profile",
		Text:         text,
		Keyboard:     profileKeyboard(state),
		NoNavigation: state != fsm.Idle && state != profileView,
	}
}

func stalePage() navigation.Page {
	return profilePage("Эта кнопка устарела. Откройте профиль заново.", fsm.Idle)
}
