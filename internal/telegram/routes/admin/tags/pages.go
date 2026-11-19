package tags

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
)

func (h *Handler) getPage(userID int64, page navigation.Page, state fsm.StateID, snapshot *session) navigation.Page {
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
		restored, err := h.service.loadAndSave(dbCtx, userID, state, cloneSession(saved))
		if err != nil {
			return navigation.Page{}, err
		}
		return h.getPage(userID, tagContent(restored), restored.state, &restored.session), nil
	}
	return page
}

func tagContent(result tagResult) navigation.Page {
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
			addTagButtons(&view, tagButton(fmt.Sprintf("№%d %s", event.ID, string(label)), "event"+strconv.FormatInt(event.ID, 10)))
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
				addTagButtons(&view, tagButton(mark+tag.Name, "toggle"+strconv.FormatInt(tag.ID, 10)))
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
		addTagButtons(&view, tagButton("⬅️", "page"+strconv.Itoa(s.page-1)))
	}
	if result.more {
		addTagButtons(&view, tagButton("➡️", "page"+strconv.Itoa(s.page+1)))
	}
	if state == browse {
		addTagButtons(&view, tagButton("Создать тег", "create"), tagButton("Назначить теги мероприятию", "assign"))
	}
	if state == editTags {
		addTagButtons(&view, tagButton("Снять все теги", "clear"), tagButton("Сохранить", "save"))
	}
	if view.Keyboard != nil {
		keyboards.WithScreenToken(view.Keyboard, s.Token)
	}
	return view
}
