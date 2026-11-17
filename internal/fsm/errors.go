package fsm

import "errors"

var (
	ErrUserStateNotInitialized = errors.New("user state is not initialized")
	ErrUserDataNotFound        = errors.New("user data not found")
)
