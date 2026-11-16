package tags

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (f *Flow) render(ctx context.Context, s *session) (screen, error) {
	view := screen{}
	if s.mode == createName {
		return screen{text: "Введите название нового тега.", buttons: []button{{"Отменить", "admin:tags"}}}, nil
	}
	more := false
	if s.mode == chooseEvent {
		events, err := f.events.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return screen{}, err
		}
		more = len(events) > pageSize
		if more {
			events = events[:pageSize]
		}
		s.events = events
		view.text = "Выберите мероприятие для назначения тегов."
		if len(events) == 0 {
			view.text = "Мероприятий пока нет."
		}
		for _, event := range events {
			label := []rune(event.Title)
			if len(label) > 45 {
				label = append(label[:45], '…')
			}
			view.buttons = append(view.buttons, s.button(fmt.Sprintf("№%d %s", event.ID, string(label)), "event"+strconv.FormatInt(event.ID, 10)))
		}
	} else {
		tags, err := f.repo.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return screen{}, err
		}
		more = len(tags) > pageSize
		if more {
			tags = tags[:pageSize]
		}
		s.tags = tags
		if s.mode == editTags {
			view.text = fmt.Sprintf("Теги мероприятия №%d\nНажимайте на теги для выбора. Выбрано: %d.\nИзменения применяются после сохранения.", s.event.ID, len(s.selected))
			for _, tag := range tags {
				mark := "○ "
				if s.selected[tag.ID] {
					mark = "✓ "
				}
				view.buttons = append(view.buttons, s.button(mark+tag.Name, "toggle"+strconv.FormatInt(tag.ID, 10)))
			}
		} else {
			var names []string
			for _, tag := range tags {
				names = append(names, "• "+tag.Name)
			}
			view.text = "Теги\n\n" + strings.Join(names, "\n")
		}
		if len(tags) == 0 {
			if s.mode == browse {
				view.text += "\nТегов пока нет. Нажмите «Создать тег»."
			} else {
				view.text += "\nТегов пока нет. Вернитесь назад и создайте тег."
			}
		}
	}
	if s.page > 0 {
		view.buttons = append(view.buttons, s.button("← Назад", "page"+strconv.Itoa(s.page-1)))
	}
	if more {
		view.buttons = append(view.buttons, s.button("Далее →", "page"+strconv.Itoa(s.page+1)))
	}
	if s.mode == browse {
		view.buttons = append(view.buttons, s.button("Создать тег", "create"), s.button("Назначить теги мероприятию", "assign"))
	}
	if s.mode == editTags {
		view.buttons = append(view.buttons, s.button("Снять все теги", "clear"), s.button("Сохранить", "save"))
	}
	view.text += fmt.Sprintf("\nСтраница %d", s.page+1)
	return view, nil
}
