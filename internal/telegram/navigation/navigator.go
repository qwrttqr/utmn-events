package navigation

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type HistoryMode uint8

const (
	HistoryPush HistoryMode = iota + 1
	HistoryMatchCurrent
)

type Page struct {
	Key          string
	Text         string
	PhotoFileID  string
	Keyboard     *models.InlineKeyboardMarkup
	Admin        bool
	Root         bool
	NoNavigation bool
	ClearHistory bool
	History      HistoryMode
	Restore      func(context.Context) (Page, error)
}

type session struct {
	history  []Page
	messages []int
	token    string
	text     string
	keyboard *models.InlineKeyboardMarkup
}

type request struct {
	navigator *Navigator
	update    *models.Update
}
type contextKey struct{}

type Navigator struct {
	mu       sync.Mutex
	sessions map[int64]*session
	adminIDs []int64
	home     func(int64) Page
	cleanup  func(int64)
}

func New(adminIDs []int64, home func(int64) Page) *Navigator {
	return &Navigator{sessions: make(map[int64]*session), adminIDs: slices.Clone(adminIDs), home: home}
}

func (n *Navigator) SetCleanup(cleanup func(int64)) { n.cleanup = cleanup }

func (n *Navigator) Middleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		next(context.WithValue(ctx, contextKey{}, request{n, update}), b, update)
	}
}

func (n *Navigator) Register(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "nav:", bot.MatchTypePrefix, n.handle)
}

func Show(ctx context.Context, b *bot.Bot, chatID int64, page Page) error {
	r, managed := ctx.Value(contextKey{}).(request)
	if !managed || chatID <= 0 {
		_, err := render(ctx, b, chatID, page, page.Keyboard)
		return err
	}
	n := r.navigator
	n.mu.Lock()
	defer n.mu.Unlock()
	if page.Admin && !slices.Contains(n.adminIDs, chatID) {
		return errors.New("admin page access denied")
	}
	s := n.sessions[chatID]
	if s == nil {
		s = &session{}
	}
	history := slices.Clone(s.history)
	if page.Root {
		history = []Page{page}
	} else {
		if page.ClearHistory && len(history) > 0 {
			history = history[:1]
		}
		if len(history) == 0 && n.home != nil {
			history = append(history, n.home(chatID))
		}
		history = recordPage(history, page)
		if len(history) > 10 {
			history = append(history[:1:1], history[len(history)-9:]...)
		}
	}
	if err := n.display(ctx, b, chatID, s, page, history, r.update); err != nil {
		return err
	}
	n.sessions[chatID] = s
	return nil
}

func recordPage(history []Page, page Page) []Page {
	switch page.History {
	case HistoryPush:
		return append(history, page)
	case HistoryMatchCurrent:
		if last := len(history) - 1; last >= 0 && history[last].Key == page.Key {
			history[last] = page
			return history
		}
	default:
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Key == page.Key {
				history[i] = page
				return history[:i+1]
			}
		}
	}
	return append(history, page)
}

func (n *Navigator) handle(ctx context.Context, b *bot.Bot, update *models.Update) {
	q := update.CallbackQuery

	if q == nil || q.Message.Message == nil {
		return
	}

	message := q.Message.Message

	if message.Chat.Type != models.ChatTypePrivate || message.Chat.ID != q.From.ID {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	s := n.sessions[message.Chat.ID]

	parts := strings.Split(q.Data, ":")
	answer := ""

	if s == nil || len(s.history) == 0 {
		answer = "История истекла. Открываю главное меню."
	} else if len(parts) != 3 || parts[1] != s.token {
		answer = "Эта кнопка устарела."
	} else if s.history[len(s.history)-1].Admin && !slices.Contains(n.adminIDs, q.From.ID) {
		answer = "Только для администратора."
	}

	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID, Text: answer}); err != nil {
		slog.ErrorContext(ctx, "answer navigation callback failed", "error", err)
	}

	if (s == nil || len(s.history) == 0) && n.home != nil {
		page := n.home(q.From.ID)
		if page.Admin && !slices.Contains(n.adminIDs, q.From.ID) {
			return
		}
		if n.cleanup != nil {
			n.cleanup(q.From.ID)
		}
		s = &session{}
		if err := n.display(ctx, b, message.Chat.ID, s, page, []Page{page}, update); err != nil {
			slog.ErrorContext(ctx, "open navigation home failed", "error", err)
		} else {
			n.sessions[message.Chat.ID] = s
		}
		return
	}

	if answer != "" {
		return
	}
	history := slices.Clone(s.history)

	switch parts[2] {
	case "home":
		history = history[:1]
	case "back":
		if len(history) > 1 {
			history = history[:len(history)-1]
		}
	case "refresh":
	default:
		return
	}

	page := history[len(history)-1]

	if page.Admin && !slices.Contains(n.adminIDs, q.From.ID) {
		return
	}
	if page.Restore != nil {
		var err error
		page, err = page.Restore(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "restore navigation page failed", "error", err)
			return
		}
		history[len(history)-1] = page
	} else if n.cleanup != nil {
		n.cleanup(q.From.ID)
	}

	if err := n.display(ctx, b, message.Chat.ID, s, page, history, update); err != nil {
		slog.ErrorContext(ctx, "navigate failed", "error", err)
	}
}

func (n *Navigator) display(ctx context.Context, b *bot.Bot, chatID int64, s *session, page Page, history []Page, update *models.Update) error {
	token := rand.Text()
	markup := cloneKeyboard(page.Keyboard)
	if !page.NoNavigation {
		if len(history) > 1 {
			// Home already leads to the panel root; remove duplicate menu buttons.
			var rows [][]models.InlineKeyboardButton
			for _, row := range markup.InlineKeyboard {
				var kept []models.InlineKeyboardButton
				for _, button := range row {
					if button.CallbackData != history[0].Key {
						kept = append(kept, button)
					}
				}
				if len(kept) > 0 {
					rows = append(rows, kept)
				}
			}
			markup.InlineKeyboard = rows
			markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{
				{Text: "⬅️", CallbackData: "nav:" + token + ":back"},
				{Text: "🏠", CallbackData: "nav:" + token + ":home"},
			})
		}
		markup.InlineKeyboard = append(markup.InlineKeyboard, []models.InlineKeyboardButton{{Text: "🔄", CallbackData: "nav:" + token + ":refresh"}})
	}
	old := slices.Clone(s.messages)
	if len(old) == 0 && update != nil && update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil {
		old = []int{update.CallbackQuery.Message.Message.ID}
	}
	var ids []int
	text := page.Text
	if len(old) == 1 && page.PhotoFileID == "" && len([]rune(text)) <= 2000 {
		if text == s.text && reflect.DeepEqual(markup, s.keyboard) {
			ids = old
		} else {
			_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{ChatID: chatID, MessageID: old[0], Text: text, ReplyMarkup: markup})
			if err == nil {
				ids = old
			} else if !errors.Is(err, bot.ErrorBadRequest) {
				return fmt.Errorf("edit navigation screen: %w", err)
			}
		}
	}
	if len(ids) == 0 {
		var err error
		ids, err = render(ctx, b, chatID, page, markup)
		if err != nil {
			for _, id := range ids {
				erase(ctx, b, chatID, id)
			}
			return err
		}
		for _, id := range old {
			erase(ctx, b, chatID, id)
		}
	}
	s.history, s.messages, s.token, s.text, s.keyboard = history, ids, token, text, markup
	if update != nil && update.Message != nil && update.Message.Chat.ID == chatID {
		erase(ctx, b, chatID, update.Message.ID)
	}
	return nil
}

func cloneKeyboard(keyboard *models.InlineKeyboardMarkup) *models.InlineKeyboardMarkup {
	result := &models.InlineKeyboardMarkup{}
	if keyboard != nil {
		for _, row := range keyboard.InlineKeyboard {
			result.InlineKeyboard = append(result.InlineKeyboard, slices.Clone(row))
		}
	}
	return result
}

func render(ctx context.Context, b *bot.Bot, chatID int64, page Page, markup *models.InlineKeyboardMarkup) ([]int, error) {
	var ids []int
	if page.PhotoFileID != "" {
		message, err := b.SendPhoto(ctx, &bot.SendPhotoParams{ChatID: chatID, Photo: &models.InputFileString{Data: page.PhotoFileID}})
		if err != nil {
			return ids, fmt.Errorf("send navigation photo: %w", err)
		}
		ids = append(ids, message.ID)
	}
	remaining := []rune(page.Text)
	for len(remaining) > 0 {
		size := min(len(remaining), 2000)
		params := &bot.SendMessageParams{ChatID: chatID, Text: string(remaining[:size])}
		remaining = remaining[size:]
		if len(remaining) == 0 {
			params.ReplyMarkup = markup
		}
		message, err := b.SendMessage(ctx, params)
		if err != nil {
			return ids, fmt.Errorf("send navigation screen: %w", err)
		}
		ids = append(ids, message.ID)
	}
	return ids, nil
}

func erase(ctx context.Context, b *bot.Bot, chatID int64, id int) {
	if _, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: id}); err != nil {
		slog.DebugContext(ctx, "delete previous navigation message failed", "message_id", id, "error", err)
	}
}
