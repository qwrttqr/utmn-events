package events

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/telegram/navigation"
	"utmn-events-bot/internal/telegram/pagination"
)

func entryKeyboard() *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Мероприятия", CallbackData: "user:events"}}}}
}

func (f *Flow) render(ctx context.Context, userID int64, s *session) (navigation.Page, error) {
	markup := &models.InlineKeyboardMarkup{}
	callback := func(action string) string { return "user:event:" + s.token + ":" + action }
	add := func(text, action string) {
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: text, CallbackData: callback(action)}})
	}
	page := navigation.Page{Key: fmt.Sprintf("user:events:%d:%d", s.mode, s.eventID), Keyboard: markup}
	switch s.mode {
	case chooseTags:
		tags, err := f.tags.List(ctx, pagination.Size+1, s.page*pagination.Size)
		if err != nil {
			return navigation.Page{}, err
		}
		more := len(tags) > pagination.Size
		if more {
			tags = tags[:pagination.Size]
		}
		s.tags = tags
		page.Text = fmt.Sprintf("Выберите хотя бы один тег.\nСовпадение по любому выбранному тегу.\nВыбрано: %d\nСтраница %d", len(s.selected), s.page+1)
		if len(tags) == 0 {
			page.Text += "\nТегов пока нет."
		}
		for _, tag := range tags {
			mark := "[ ] "
			if _, selected := s.selected[tag.ID]; selected {
				mark = "[x] "
			}
			add(mark+tag.Name, "toggle"+strconv.FormatInt(tag.ID, 10))
		}
		pagination.Add(markup, s.page, more, callback)
		if len(s.selected) > 0 {
			add("Снять выбор", "clear")
		}
		add("Найти мероприятия", "search")
	case all, mine, results:
		filter := domain.EventFilter{UpcomingOnly: s.mode != mine}
		label := "Предстоящие мероприятия"
		if s.mode == mine {
			filter.UserTelegramID, label = userID, "Мои записи"
		}
		if s.mode == results {
			if len(s.selected) == 0 {
				return navigation.Page{}, fmt.Errorf("tag search requires at least one tag")
			}
			var names []string
			for id, name := range s.selected {
				filter.TagIDs = append(filter.TagIDs, id)
				names = append(names, name)
			}
			slices.Sort(filter.TagIDs)
			slices.Sort(names)
			label = "Мероприятия по тегам: " + strings.Join(names, "; ")
		}
		events, err := f.repo.Browse(ctx, filter, pagination.Size+1, s.page*pagination.Size)
		if err != nil {
			return navigation.Page{}, err
		}
		more := len(events) > pagination.Size
		if more {
			events = events[:pagination.Size]
		}
		page.Text = fmt.Sprintf("%s\nСтраница %d", label, s.page+1)
		if len(events) == 0 {
			page.Text += "\nМероприятий не найдено."
		}
		for _, event := range events {
			title := []rune(event.Title)
			if len(title) > 45 {
				title = append(title[:45], '…')
			}
			add(fmt.Sprintf("№%d %s", event.ID, string(title)), "open"+strconv.FormatInt(event.ID, 10))
		}
		pagination.Add(markup, s.page, more, callback)
	case card, cancelEnrollment:
		event, err := f.repo.Get(ctx, s.eventID)
		if err != nil {
			return navigation.Page{}, err
		}
		enrolled, err := f.registrations.IsEnrolled(ctx, s.eventID, userID)
		if err != nil {
			return navigation.Page{}, err
		}
		if s.mode == cancelEnrollment {
			page.Text = fmt.Sprintf("Отменить вашу запись на «%s» (№%d)?", event.Title, event.ID)
			add("Подтвердить отмену записи", "confirm")
			add("Оставить запись", "keep")
		} else {
			date := func(value time.Time) string {
				if value.IsZero() {
					return "не указано"
				}
				return value.In(f.location).Format("02.01.2006 15:04")
			}
			capacity := "без ограничений"
			if event.Capacity != nil {
				capacity = strconv.Itoa(int(*event.Capacity))
			}
			page.Text = fmt.Sprintf("%s (№%d)\n\n%s\n\nМесто: %s\nНачало: %s\nОкончание: %s\nЗаписано: %d\nМест: %s\nТеги: %s", event.Title, event.ID, event.Description, event.Location, date(event.StartsAt), date(event.EndsAt), event.RegistrationCount, capacity, strings.Join(event.Tags, "; "))
			page.PhotoFileID = event.PhotoFileID
			if enrolled {
				page.Text += "\n\nВы записаны на это мероприятие."
				add("Отменить запись", "cancel")
			} else if !event.StartsAt.After(f.now()) {
				page.Text += "\n\nЗапись закрыта."
			} else if event.Capacity != nil && event.RegistrationCount >= int64(*event.Capacity) {
				page.Text += "\n\nСвободных мест нет."
			} else {
				add("Записаться", "enroll")
			}
		}
	}
	saved := cloneSession(*s)
	page.Restore = func(ctx context.Context) (navigation.Page, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		copy := cloneSession(saved)
		copy.token = rand.Text()
		dbCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		page, err := f.render(dbCtx, userID, &copy)
		if err != nil {
			return navigation.Page{}, err
		}
		f.sessions[userID] = copy
		return page, nil
	}
	return page, nil
}
