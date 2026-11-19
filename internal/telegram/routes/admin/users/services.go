package users

import (
	"context"
	"slices"

	"utmn-events-bot/internal/domain"
	"utmn-events-bot/internal/fsm"
	"utmn-events-bot/internal/telegram/keyboards"
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
	keyboards.TokenizedDraft
}
type listResult struct {
	users      []domain.User
	pageNumber int
	more       bool
	keyboards.TokenizedDraft
}

type UsersService struct {
	machine  *fsm.FSM
	repo     Repository
	adminIDs []int64
}

const (
	usersList      fsm.StateID = "admin.users.list"
	usersDetails   fsm.StateID = "admin.users.details"
	usersPromotion fsm.StateID = "admin.users.promotion"
)

func NewService(machine *fsm.FSM, repo Repository, adminIDs []int64) *UsersService {
	return &UsersService{machine: machine, repo: repo, adminIDs: slices.Clone(adminIDs)}
}

func (f *UsersService) page(ctx context.Context, userID int64, current listResult, pageNumber int) (listResult, error) {
	if pageNumber < 0 || (pageNumber != current.pageNumber-1 && pageNumber != current.pageNumber+1) {
		pageNumber = current.pageNumber
	}
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
	result.RotateScreenToken()
	f.machine.Replace(userID, usersList, result)
	return result, nil
}

func (f *UsersService) selectUser(ctx context.Context, userID, targetID int64, current listResult) (userSelection, error) {
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
	selection := userSelection{target: user, pageNumber: pageNumber}
	selection.RotateScreenToken()
	f.machine.Replace(userID, usersDetails, selection)
	return selection, nil
}

func (f *UsersService) beginPromotion(userID int64, selection userSelection) userSelection {
	selection.RotateScreenToken()
	f.machine.Replace(userID, usersPromotion, selection)
	return selection
}
