package tags

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/navigation"
)

func (h *Handler) getPage(userID int64, result tagResult, state fsm.StateID, snapshot *session) navigation.Page {
	page := tagContent(result)
	page.Key = "admin:tags:result"
	page.Admin = true
	page.ClearHistory = page.ClearHistory || snapshot == nil
	if page.Keyboard == nil {
		page.Keyboard = tagKeyboard()
	}
	if snapshot == nil {
		return page
	}
	saved := cloneSession(*snapshot)
	page.Key = fmt.Sprintf("admin:tags:%s:%d", state, saved.event.ID)
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		restored, err := h.service.restore(dbCtx, userID, state, saved)
		if err != nil {
			return navigation.Page{}, err
		}
		return h.getPage(userID, restored, restored.state, &restored.session), nil
	}
	return page
}

func tagContent(result tagResult) navigation.Page {
	switch result.status {
	case tagsLoadFailed:
		return navigation.Page{Text: "Не удалось загрузить теги.", Keyboard: tagKeyboard(button{"Повторить", "admin:tags"}, button{"В меню", "admin:menu"})}
	case tagSessionMissing:
		return navigation.Page{Text: "Откройте раздел тегов заново.", Keyboard: tagKeyboard(button{"Теги", "admin:tags"})}
	case tagActionStale:
		return navigation.Page{Text: "Эта кнопка устарела.", Keyboard: tagKeyboard(button{"Теги", "admin:tags"})}
	case tagAssignmentsSaved:
		return navigation.Page{Text: "Теги мероприятия сохранены.", ClearHistory: true, Keyboard: tagKeyboard(button{"К тегам", "admin:tags"}, button{"В меню", "admin:menu"})}
	case tagNameInvalid:
		return navigation.Page{Text: "Введите один тег: от 1 до 40 символов, без команд, точки с запятой и переносов строк.", Keyboard: tagKeyboard(button{"Отменить", "admin:menu"})}
	case tagCreated:
		s := result.session
		return navigation.Page{Text: fmt.Sprintf("Тег «%s» сохранён.", result.name), ClearHistory: true, Keyboard: tagKeyboard(s.button("Создать ещё", "create"), s.button("Назначить мероприятию", "assign"), button{"К тегам", "admin:tags"})}
	case tagActionFailed:
		page := navigation.Page{Text: "Не удалось обратиться к базе данных. Попробуйте ещё раз."}
		if result.retryAction != "" {
			s := session{token: result.retryToken}
			addTagButtons(&page, s.button("Повторить", result.retryAction))
		} else {
			page.Text += "\nОтправьте название тега повторно."
		}
		addTagButtons(&page, button{"Отменить", "admin:menu"})
		return page
	}
	state, s := result.state, result.session
	view := navigation.Page{}
	if state == createName {
		return navigation.Page{Text: "Введите название нового тега.", Keyboard: tagKeyboard(button{"Отменить", "admin:menu"})}
	}
	if state == chooseEvent {
		events := s.events
		view.Text = "Выберите мероприятие для назначения тегов."
		if len(events) == 0 {
			view.Text = "Мероприятий пока нет."
		}
		for _, event := range events {
			label := []rune(event.Title)
			if len(label) > 45 {
				label = append(label[:45], '…')
			}
			addTagButtons(&view, s.button(fmt.Sprintf("№%d %s", event.ID, string(label)), "event"+strconv.FormatInt(event.ID, 10)))
		}
	} else {
		tags := s.tags
		if state == editTags {
			view.Text = fmt.Sprintf("Теги мероприятия №%d\nНажимайте на теги для выбора. Выбрано: %d.\nИзменения применяются после сохранения.", s.event.ID, len(s.selected))
			for _, tag := range tags {
				mark := "○ "
				if s.selected[tag.ID] {
					mark = "✓ "
				}
				addTagButtons(&view, s.button(mark+tag.Name, "toggle"+strconv.FormatInt(tag.ID, 10)))
			}
		} else {
			var names []string
			for _, tag := range tags {
				names = append(names, "• "+tag.Name)
			}
			view.Text = "Теги\n\n" + strings.Join(names, "\n")
		}
		if len(tags) == 0 {
			if state == browse {
				view.Text += "\nТегов пока нет. Нажмите «Создать тег»."
			} else {
				view.Text += "\nТегов пока нет. Вернитесь назад и создайте тег."
			}
		}
	}
	if s.page > 0 {
		addTagButtons(&view, s.button("⬅️", "page"+strconv.Itoa(s.page-1)))
	}
	if result.more {
		addTagButtons(&view, s.button("➡️", "page"+strconv.Itoa(s.page+1)))
	}
	if state == browse {
		addTagButtons(&view, s.button("Создать тег", "create"), s.button("Назначить теги мероприятию", "assign"))
	}
	if state == editTags {
		addTagButtons(&view, s.button("Снять все теги", "clear"), s.button("Сохранить", "save"))
	}
	return view
}
