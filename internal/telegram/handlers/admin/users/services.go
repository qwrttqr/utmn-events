package users

import (
	"context"
	"slices"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/pagination"
)

type Repository interface {
	List(context.Context, int, int) ([]domain.User, error)
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

	state, pageNumber, exists := fsm.Load[int](f.machine, userID)
	if !exists || state != usersList {
		return listResult{expired: true}, nil
	}
	pageNumber, _ = pagination.Move(pageNumber, action)
	return f.loadPage(ctx, userID, pageNumber)
}

func (f *UsersService) list(ctx context.Context, userID int64) (listResult, error) {
	return f.loadPage(ctx, userID, 0)
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

	f.machine.Replace(userID, usersList, pageNumber)
	return listResult{users: users, pageNumber: pageNumber, more: more}, nil
}

func (f *UsersService) restore(ctx context.Context, userID int64, pageNumber int) (listResult, error) {
	return f.loadPage(ctx, userID, pageNumber)
}

const usersList fsm.StateID = "admin.users.list"
