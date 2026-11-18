package registration

import (
	"fmt"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/navigation"
)

func missingRegistrationPage() navigation.Page {
	return registrationPage("Начните регистрацию.", registrationButton("Начать регистрацию", "registration:start"))
}

func cancelledRegistrationPage() navigation.Page {
	return registrationPage("Для доступа к боту необходимо зарегистрироваться.", registrationButton("Начать регистрацию", "registration:start"))
}

func saveErrorPage(d draft) navigation.Page {
	page := confirmationPage(d)
	page.Text = "Не удалось обратиться к базе данных. Попробуйте ещё раз.\n\n" + page.Text
	page.Root = false
	return page
}

func invalidNamePage(state fsm.StateID, d draft) navigation.Page {
	page := promptPage(state, d)
	page.Text = "Введите от 1 до 100 символов без команд и переносов строк.\n\n" + page.Text
	return page
}

func promptPage(state fsm.StateID, d draft) navigation.Page {
	switch state {
	case firstName:
		return firstNamePage()
	case lastName:
		return lastNamePage()
	case surname:
		return surnamePage()
	case registrationConfirmation:
		return confirmationPage(d)
	default:
		return missingRegistrationPage()
	}
}

func firstNamePage() navigation.Page {
	return registrationPage("Для доступа к боту необходимо зарегистрироваться.\nВведите ваше имя.", cancelButton())
}

func lastNamePage() navigation.Page {
	return registrationPage("Введите вашу фамилию.", cancelButton())
}

func surnamePage() navigation.Page {
	return registrationPage("Введите ваше отчество или нажмите «Пропустить».", registrationButton("Пропустить", "registration:skip"), cancelButton())
}

func confirmationPage(d draft) navigation.Page {
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
