package users

import (
	"context"
	"crypto/rand"
	"slices"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/pagination"
)

type Repository interface {
	List(context.Context, int, int) ([]domain.User, error)
	GetByTelegramID(context.Context, int64) (domain.User, error)
	GrantAdmin(context.Context, int64) error
}

type userSelection struct {
	target     domain.User
	pageNumber int
	token      string
}
type listResult struct {
	users      []domain.User
	pageNumber int
	more       bool
	expired    bool
}

type UsersService struct {
	machine  *fsm.FSM
	repo     Repository
	adminIDs []int64
}

func NewService(machine *fsm.FSM, repo Repository, adminIDs []int64) *UsersService {
	return &UsersService{machine: machine, repo: repo, adminIDs: slices.Clone(adminIDs)}
}

func (f *UsersService) page(ctx context.Context, userID int64, action string) (listResult, error) {
	state, current, exists := fsm.GetDraft[listResult](f.machine, userID)
	if !exists || state != usersList {
		return listResult{expired: true}, nil
	}
	pageNumber, _ := pagination.Move(current.pageNumber, action)
	return f.loadPage(ctx, userID, pageNumber)
}

func (f *UsersService) loadPage(ctx context.Context, userID int64, pageNumber int) (listResult, error) {
	offset := pageNumber * pagination.Size
	users, err := f.repo.List(ctx, pagination.Size+1, offset)
	if err != nil {
		return listResult{}, err
	}
	more := len(users) > pagination.Size
	if more {
		users = users[:pagination.Size]
	}

	result := listResult{users: users, pageNumber: pageNumber, more: more}
	f.machine.Replace(userID, usersList, result)
	return result, nil
}

func (f *UsersService) selectUser(ctx context.Context, userID, targetID int64) (userSelection, error) {
	state, current, exists := fsm.GetDraft[listResult](f.machine, userID)
	if !exists || state != usersList {
		return userSelection{}, nil
	}
	for _, user := range current.users {
		if user.TelegramID == targetID {
			return f.loadUser(ctx, userID, targetID, current.pageNumber)
		}
	}
	return userSelection{}, nil
}

func (f *UsersService) loadUser(ctx context.Context, userID, targetID int64, pageNumber int) (userSelection, error) {
	user, err := f.repo.GetByTelegramID(ctx, targetID)
	if err != nil {
		return userSelection{}, err
	}
	selection := userSelection{target: user, pageNumber: pageNumber, token: rand.Text()}
	f.machine.Replace(userID, usersDetails, selection)
	return selection, nil
}

func (f *UsersService) selection(userID int64, token string, expected fsm.StateID) (userSelection, bool) {
	state, selected, exists := fsm.GetDraft[userSelection](f.machine, userID)
	return selected, exists && state == expected && selected.token == token
}

const (
	usersList      fsm.StateID = "admin.users.list"
	usersDetails   fsm.StateID = "admin.users.details"
	usersPromotion fsm.StateID = "admin.users.promotion"
)
