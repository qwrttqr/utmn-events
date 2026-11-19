package profile

import (
	"fmt"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
)

func (h *Handler) getPage(result profileResult) navigation.Page {
	if result.state == fsm.Idle || result.state == "" {
		return stalePage()
	}
	page := render(result.state, result.draft)
	if result.message != "" {
		page.Text = result.message + "\n\n" + page.Text
	}
	return page
}

func render(state fsm.StateID, d draft) navigation.Page {
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
	page := profilePage(text, state)
	keyboards.WithScreenToken(page.Keyboard, d.Token)
	return page
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
