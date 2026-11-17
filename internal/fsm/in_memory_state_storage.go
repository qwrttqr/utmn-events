package fsm

import "context"

type UserStateInMemoryStorage struct {
	store *ShardedInMemoryStore[int64, StateID]
}

var _ UserStateStorage = (*UserStateInMemoryStorage)(nil)

func NewUserStateInMemoryStorage(shardCount uint32, hash func(int64) uint32) *UserStateInMemoryStorage {
	if hash == nil {
		panic("fsm: hash function must not be nil")
	}
	return &UserStateInMemoryStorage{store: NewCache[int64, StateID](shardCount, hash)}
}

func (s *UserStateInMemoryStorage) Set(_ context.Context, userID int64, stateID StateID) error {
	s.store.Set(userID, stateID)
	return nil
}

func (s *UserStateInMemoryStorage) Get(_ context.Context, userID int64) (StateID, error) {
	val, ok := s.store.Get(userID)
	if !ok {
		var zero StateID
		return zero, ErrUserStateNotInitialized
	}
	return val, nil
}
