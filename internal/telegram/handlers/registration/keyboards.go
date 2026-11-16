package registration

import "github.com/go-telegram/bot/models"

func keyboard(view screen) *models.InlineKeyboardMarkup {
	markup := &models.InlineKeyboardMarkup{}
	add := func(text, data string) {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{
			{Text: text, CallbackData: data},
		})
	}
	if view.start {
		add("Начать регистрацию", "registration:start")
	}
	if view.skip {
		add("Пропустить", "registration:skip")
	}
	if view.confirm {
		add("Подтвердить", "registration:confirm")
		add("Заполнить заново", "registration:restart")
	}
	if view.cancel {
		add("Отменить регистрацию", "registration:cancel")
	}
	if view.menu {
		add("Открыть меню", "home:menu")
	}
	if len(markup.InlineKeyboard) == 0 {
		return nil
	}
	return markup
}
