package fsm

import "context"

type UserDataInMemoryStorage[K comparable, V any] struct {
	store *ShardedInMemoryStore[int64, map[K]V]
}

func NewUserDataInMemoryStorage[K comparable, V any](shardCount uint32, hash func(int64) uint32) *UserDataInMemoryStorage[K, V] {
	store := NewCache[int64, map[K]V](shardCount, hash)
	storage := &UserDataInMemoryStorage[K, V]{store: store}
	var _ DataStorage[K, V] = storage
	return storage
}

func (s *UserDataInMemoryStorage[K, V]) Set(_ context.Context, userID int64, key K, value V) error {
	s.store.Update(userID, func(data map[K]V, _ bool) map[K]V {
		if data == nil {
			data = make(map[K]V)
		}
		data[key] = value
		return data
	})
	return nil
}

func (s *UserDataInMemoryStorage[K, V]) Get(_ context.Context, userID int64, key K) (V, error) {
	var value V
	var ok bool
	s.store.Read(userID, func(data map[K]V, _ bool) {
		value, ok = data[key]
	})
	if !ok {
		var zero V
		return zero, ErrUserDataNotFound
	}
	return value, nil
}

func (s *UserDataInMemoryStorage[K, V]) Delete(_ context.Context, userID int64, key K) error {
	s.store.Update(userID, func(data map[K]V, _ bool) map[K]V {
		delete(data, key)
		return data
	})
	return nil
}

func (s *UserDataInMemoryStorage[K, V]) Clear(_ context.Context, userID int64) error {
	s.store.Delete(userID)
	return nil
}
