package registration

import (
	"fmt"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/navigation"
)

func resultPage(result registrationResult) navigation.Page {
	switch result.status {
	case registrationMissing:
		return registrationPage("Начните регистрацию.", registrationButton("Начать регистрацию", "registration:start"))
	case registrationCancelled:
		return registrationPage("Для доступа к боту необходимо зарегистрироваться.", registrationButton("Начать регистрацию", "registration:start"))
	case registrationCompleted:
		return registrationPage("Регистрация завершена!", registrationButton("🏠", "home:menu"))
	case registrationInvalidName:
		page := prompt(result.state, result.draft)
		page.Text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + page.Text
		return page
	}

	return prompt(result.state, result.draft)
}

func prompt(state fsm.StateID, d draft) navigation.Page {
	switch state {
	case firstName:
		return registrationPage("Для доступа к боту необходимо зарегистрироваться.\nВведите ваше имя.", cancelButton())
	case lastName:
		return registrationPage("Введите вашу фамилию.", cancelButton())
	case surname:
		return registrationPage("Введите ваше отчество или нажмите «Пропустить».", registrationButton("Пропустить", "registration:skip"), cancelButton())
	default:
		name := d.profile.Surname
		if name == "" {
			name = "не указано"
		}
		return registrationPage(
			fmt.Sprintf("Проверьте данные:\nИмя: %s\nФамилия: %s\nОтчество: %s", d.profile.FirstName, d.profile.LastName, name),
			registrationButton("Подтвердить", "registration:confirm"),
			registrationButton("Заполнить заново", "registration:restart"),
			cancelButton(),
		)
	}
}
