package manage

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/pagination"
	"utmn-events-bot/internal/telegram/presentation"
)

func listKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Мероприятия", CallbackData: "admin:events"}}}}
}

func (f *Flow) render(ctx context.Context, userID int64, s session) (navigation.Page, error) {
	keyboard := &models.InlineKeyboardMarkup{}
	add := func(text, action string) {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: "admin:manage:" + s.token + ":" + action}})
	}
	page := navigation.Page{Key: fmt.Sprintf("admin:events:%d:%d", s.mode, s.eventID), Admin: true, Keyboard: keyboard}
	if s.mode == list {
		page.Key, page.Text = "admin:events:list", fmt.Sprintf("Мероприятия\nСтраница %d\nВыберите мероприятие.", s.page+1)
		events, err := f.repo.List(ctx, pageSize+1, s.page*pageSize)
		if err != nil {
			return navigation.Page{}, err
		}
		more := len(events) > pageSize
		if more {
			events = events[:pageSize]
		}
		if len(events) == 0 {
			page.Text = "Мероприятий пока нет."
		}
		for _, event := range events {
			title := []rune(event.Title)
			if len(title) > 45 {
				title = append(title[:45], '…')
			}
			add(fmt.Sprintf("№%d %s", event.ID, string(title)), "open"+strconv.FormatInt(event.ID, 10))
		}
		pagination.Add(keyboard, s.page, more, func(action string) string { return "admin:manage:" + s.token + ":" + action })
	} else {
		event, err := f.repo.Get(ctx, s.eventID)
		if err != nil {
			return navigation.Page{}, err
		}
		switch s.mode {
		case card:
			location, err := time.LoadLocation("Asia/Yekaterinburg")
			if err != nil {
				return navigation.Page{}, err
			}
			date := func(t time.Time) string {
				if t.IsZero() {
					return "не указано"
				}
				return t.In(location).Format("02.01.2006 15:04")
			}
			capacity := "без ограничений"
			if event.Capacity != nil {
				capacity = strconv.Itoa(int(*event.Capacity))
			}
			page.Text = fmt.Sprintf("%s (№%d)\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nЗарегистрировано: %d\nМест: %s\nТеги: %s", event.Title, event.ID, event.Description, event.Location, date(event.StartsAt), date(event.EndsAt), event.RegistrationCount, capacity, strings.Join(event.Tags, "; "))
			page.PhotoFileID = event.PhotoFileID
			add("Редактировать", "edit")
			add("Удалить", "delete")
			add(fmt.Sprintf("Участники (%d)", event.RegistrationCount), "participants")
		case participants:
			users, err := f.users.ListForEvent(ctx, s.eventID, pageSize+1, s.page*pageSize)
			if err != nil {
				return navigation.Page{}, err
			}
			more := len(users) > pageSize
			if more {
				users = users[:pageSize]
			}
			page.Text = fmt.Sprintf("Участники «%s» (№%d)\nЗарегистрировано: %d\nСтраница %d\n\n%s", event.Title, event.ID, event.RegistrationCount, s.page+1, presentation.Users(users, s.page*pageSize, f.adminIDs))
			pagination.Add(keyboard, s.page, more, func(action string) string { return "admin:manage:" + s.token + ":" + action })
		case fields:
			page.Text = fmt.Sprintf("Редактирование «%s» (№%d)\nЧто изменить?", event.Title, event.ID)
			for _, field := range []struct {
				name  string
				field domain.EventField
			}{
				{"Название", domain.EventTitle}, {"Описание", domain.EventDescription}, {"Фото", domain.EventPhoto},
				{"Место", domain.EventLocation}, {"Даты и время", domain.EventDates}, {"Количество мест", domain.EventCapacity}, {"Теги", domain.EventTags},
			} {
				add(field.name, "field"+string(field.field))
			}
		case confirmDelete:
			page.Text = fmt.Sprintf("Удалить «%s» (№%d)?\nБудут также удалены все регистрации на мероприятие (сейчас: %d).", event.Title, event.ID, event.RegistrationCount)
			add("Подтвердить удаление", "confirm")
			add("Оставить мероприятие", "keep")
		}
	}
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		s.token = rand.Text()
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		page, err := f.render(dbCtx, userID, s)
		if err != nil {
			return navigation.Page{}, err
		}
		f.sessions[userID] = s
		return page, nil
	}
	return page, nil
}
