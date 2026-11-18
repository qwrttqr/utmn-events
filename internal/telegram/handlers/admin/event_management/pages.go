package event_management

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"utmn-events-bot/internal/telegram/navigation"
)

func expiredPage() navigation.Page {
	return navigation.Page{Key: "admin:events:expired", Text: "Эта кнопка устарела. Откройте список мероприятий.", Admin: true, Keyboard: listKeyboard()}
}

func (h *Handler) page(userID int64, result managementResult) navigation.Page {
	switch result.status {
	case managementExpired:
		return expiredPage()
	case managementDeleted:
		return navigation.Page{Key: "admin:events:deleted", Text: "Мероприятие удалено.", Admin: true, ClearHistory: true, Keyboard: listKeyboard()}
	}
	if result.state == "" {
		return navigation.Page{}
	}
	state, s := result.state, result.session
	page := navigation.Page{Key: fmt.Sprintf("admin:events:%s:%d", state, s.eventID), Admin: true}
	if state == list {
		page.Key, page.Text = "admin:events:list", "Мероприятия\nВыберите мероприятие."
		events := result.events
		if len(events) == 0 {
			page.Text = "Мероприятий пока нет."
		}
		page.Keyboard = eventsKeyboard(s, events, result.more)
	} else {
		event := result.event
		switch state {
		case card:
			date := func(t time.Time) string {
				if t.IsZero() {
					return "не указано"
				}
				return t.In(result.location).Format("02.01.2006 15:04")
			}
			capacity := "без ограничений"
			if event.Capacity != nil {
				capacity = strconv.Itoa(int(*event.Capacity))
			}
			page.Text = fmt.Sprintf("%s (№%d)\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nЗарегистрировано: %d\nМест: %s\nТеги: %s", event.Title, event.ID, event.Description, event.Location, date(event.StartsAt), date(event.EndsAt), event.RegistrationCount, capacity, strings.Join(event.Tags, "; "))
			page.PhotoFileID = event.PhotoFileID
			page.Keyboard = cardKeyboard(s, event)
		case participants:
			page.Text = fmt.Sprintf("Участники «%s» (№%d)\nЗарегистрировано: %d\n\n%s", event.Title, event.ID, event.RegistrationCount, formatUsers(result.users, s.page*pageSize, h.service.adminIDs))
			page.Keyboard = participantsKeyboard(s, result.more)
		case fields:
			page.Text = fmt.Sprintf("Редактирование «%s» (№%d)\nЧто изменить?", event.Title, event.ID)
			page.Keyboard = fieldsKeyboard(s)
		case confirmDelete:
			page.Text = fmt.Sprintf("Удалить «%s» (№%d)?\nБудут также удалены все регистрации на мероприятие (сейчас: %d).", event.Title, event.ID, event.RegistrationCount)
			page.Keyboard = deleteKeyboard(s)
		}
	}
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		restored, err := h.service.restore(dbCtx, userID, state, s)
		if err != nil {
			return navigation.Page{}, err
		}
		return h.page(userID, restored), nil
	}
	return page
}
