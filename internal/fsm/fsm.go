package fsm

import (
	"context"
	"errors"
	"fmt"
)

type StateID string

type FSM[K comparable, V any] struct {
	initialStateID StateID
	statesStorage  UserStateStorage
	dataStorage    DataStorage[K, V]
}

type UserStateStorage interface {
	Set(ctx context.Context, userID int64, stateID StateID) error
	Get(ctx context.Context, userID int64) (StateID, error)
}

type DataStorage[K comparable, V any] interface {
	Set(ctx context.Context, userID int64, key K, value V) error
	Get(ctx context.Context, userID int64, key K) (V, error)
	Delete(ctx context.Context, userID int64, key K) error
	Clear(ctx context.Context, userID int64) error
}

func NewFSM[K comparable, V any](initialState StateID, statesStorage UserStateStorage, dataStorage DataStorage[K, V]) *FSM[K, V] {
	machine := &FSM[K, V]{
		initialStateID: initialState,
		statesStorage:  statesStorage,
		dataStorage:    dataStorage,
	}
	return machine
}

func (f *FSM[K, V]) CurrentState(ctx context.Context, userID int64) (StateID, error) {
	state, err := f.statesStorage.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrUserStateNotInitialized) {
			return f.initialStateID, nil
		}
		if err != nil {
			return "", fmt.Errorf("get FSM state: %w", err)
		}
	}
	return state, nil
}

func (f *FSM[K, V]) Transition(ctx context.Context, userID int64, stateID StateID) error {
	err := f.statesStorage.Set(ctx, userID, stateID)
	if err != nil {
		return fmt.Errorf("failed to set user state: %w", err)
	}
	return nil
}

func (f *FSM[K, V]) Reset(ctx context.Context, userID int64) error {
	return f.statesStorage.Set(ctx, userID, f.initialStateID)
}

func (f *FSM[K, V]) SetData(ctx context.Context, userID int64, key K, value V) error {
	err := f.dataStorage.Set(ctx, userID, key, value)
	if err != nil {
		return fmt.Errorf("failed to set user data: %w", err)
	}

	return nil
}

func (f *FSM[K, V]) GetData(ctx context.Context, userID int64, key K) (V, error) {
	v, err := f.dataStorage.Get(ctx, userID, key)
	if err != nil {
		var empty V
		return empty, fmt.Errorf("failed to get user data: %w", err)
	}

	return v, nil
}

func (f *FSM[K, V]) DeleteData(ctx context.Context, userID int64, key K) error {
	err := f.dataStorage.Delete(ctx, userID, key)
	if err != nil {
		return fmt.Errorf("failed to delete user data: %w", err)
	}

	return nil
}

func (f *FSM[K, V]) ClearData(ctx context.Context, userID int64) error {
	err := f.dataStorage.Clear(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to delete all data associated with user: %w", err)
	}
	return nil
}

func (f *FSM[K, V]) Finish(ctx context.Context, userID int64) error {
	if err := f.ClearData(ctx, userID); err != nil {
		return err
	}
	if err := f.Reset(ctx, userID); err != nil {
		return fmt.Errorf("failed to finish conversation: %w", err)
	}
	return nil
}
