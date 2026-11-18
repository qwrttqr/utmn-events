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
	GrantAdmin(context.Context, int64) error
}

type promotion struct {
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
	state, current, exists := fsm.Load[listResult](f.machine, userID)
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

func (f *UsersService) selectPromotion(userID, targetID int64) (promotion, bool) {
	state, current, exists := fsm.Load[listResult](f.machine, userID)
	if !exists || state != usersList {
		return promotion{}, false
	}
	for _, user := range current.users {
		if user.TelegramID == targetID && !isAdmin(user, f.adminIDs) {
			draft := promotion{target: user, pageNumber: current.pageNumber, token: rand.Text()}
			f.machine.Replace(userID, usersPromotion, draft)
			return draft, true
		}
	}
	return promotion{}, false
}

func (f *UsersService) pendingPromotion(userID int64, token string) (promotion, bool) {
	state, draft, exists := fsm.Load[promotion](f.machine, userID)
	return draft, exists && state == usersPromotion && draft.token == token
}

const (
	usersList      fsm.StateID = "admin.users.list"
	usersPromotion fsm.StateID = "admin.users.promotion"
)
