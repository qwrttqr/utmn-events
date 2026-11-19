package events

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"utmn-events-bot/internal/telegram/keyboards"
	"utmn-events-bot/internal/telegram/navigation"
)

func expiredPage() navigation.Page {
	return navigation.Page{Key: "user:events:expired", Text: "Эта кнопка устарела. Откройте мероприятия заново.", Keyboard: entryKeyboard()}
}

func (h *Handler) getPage(userID int64, result eventsResult) navigation.Page {
	if result.state == "" {
		return navigation.Page{}
	}
	state, s := result.state, result.session
	page := navigation.Page{Key: fmt.Sprintf("user:events:%s:%d", state, s.eventID)}
	switch state {
	case chooseTags:
		tags := s.tags
		page.Text = fmt.Sprintf("Выберите хотя бы один тег.\nСовпадение по любому выбранному тегу.\nВыбрано: %d", len(s.selected))
		if len(tags) == 0 {
			page.Text += "\nТегов пока нет."
		}
		page.Keyboard = tagsKeyboard(s, tags, result.more)
	case all, mine, results:
		label := "Предстоящие мероприятия"
		if state == mine {
			label = "Мои записи"
		}
		if state == results {
			var names []string
			for _, name := range s.selected {
				names = append(names, name)
			}
			slices.Sort(names)
			label = "Мероприятия по тегам: " + strings.Join(names, "; ")
		}
		events := result.events
		page.Text = label
		if len(events) == 0 {
			page.Text += "\nМероприятий не найдено."
		}
		page.Keyboard = eventsKeyboard(s, events, result.more)
	case card, cancelEnrollment:
		event, enrolled := result.event, result.enrolled
		page.Keyboard = cardKeyboard(state, s, event, enrolled, h.service.now())
		if state == cancelEnrollment {
			page.Text = fmt.Sprintf("Отменить вашу запись на «%s» (№%d)?", event.Title, event.ID)
		} else {
			date := func(value time.Time) string {
				if value.IsZero() {
					return "не указано"
				}
				return value.In(h.service.location).Format("02.01.2006 15:04")
			}
			capacity := "без ограничений"
			if event.Capacity != nil {
				capacity = strconv.Itoa(int(*event.Capacity))
			}
			page.Text = fmt.Sprintf("%s (№%d)\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nЗаписано: %d\nМест: %s\nТеги: %s", event.Title, event.ID, event.Description, event.Location, date(event.StartsAt), date(event.EndsAt), event.RegistrationCount, capacity, strings.Join(event.Tags, "; "))
			page.PhotoFileID = event.PhotoFileID
			if enrolled {
				page.Text += "\n\nВы записаны на это мероприятие."
			} else if !event.StartsAt.After(h.service.now()) {
				page.Text += "\n\nЗапись закрыта."
			} else if event.Capacity != nil && event.RegistrationCount >= int64(*event.Capacity) {
				page.Text += "\n\nСвободных мест нет."
			}
		}
	}
	if page.Keyboard != nil {
		keyboards.WithScreenToken(page.Keyboard, s.Token)
	}
	if result.message != "" {
		page.Text = result.message + "\n\n" + page.Text
	}

	saved := cloneSession(s)
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		restored, err := h.service.restore(dbCtx, userID, state, saved)
		if err != nil {
			return navigation.Page{}, err
		}
		return h.getPage(userID, restored), nil
	}
	return page
}
