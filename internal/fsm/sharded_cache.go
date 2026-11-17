package fsm

import "sync"

type shard[K comparable, V any] struct {
	mu      sync.Mutex
	entries map[K]V
}

type ShardedInMemoryStore[K comparable, V any] struct {
	shards []*shard[K, V]
	hash   func(K) uint32
}

func (cache *ShardedInMemoryStore[K, V]) getShard(key K) *shard[K, V] {
	return cache.shards[cache.hash(key)%uint32(len(cache.shards))]
}

func (cache *ShardedInMemoryStore[K, V]) Get(key K) (V, bool) {
	shard := cache.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	d, ok := shard.entries[key]
	return d, ok
}

func (cache *ShardedInMemoryStore[K, V]) Set(key K, value V) {
	shard := cache.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.entries[key] = value
}

func (cache *ShardedInMemoryStore[K, V]) Delete(key K) {
	shard := cache.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	delete(shard.entries, key)
}

func (cache *ShardedInMemoryStore[K, V]) Read(key K, readFn func(value V, exists bool)) {
	shard := cache.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	value, exists := shard.entries[key]
	readFn(value, exists)
}

func (cache *ShardedInMemoryStore[K, V]) Update(key K, updateFn func(currentValue V, exists bool) V) {
	shard := cache.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	currentValue, exists := shard.entries[key]
	shard.entries[key] = updateFn(currentValue, exists)
}

func NewCache[K comparable, V any](
	shardCount uint32,
	hash func(K) uint32,
) *ShardedInMemoryStore[K, V] {
	if shardCount == 0 {
		panic("fsm: shard count must be positive")
	}
	if hash == nil {
		panic("fsm: hash function must not be nil")
	}
	cache := &ShardedInMemoryStore[K, V]{
		shards: make([]*shard[K, V], shardCount),
		hash:   hash,
	}

	for i := range shardCount {
		cache.shards[i] = &shard[K, V]{entries: make(map[K]V)}
	}

	return cache
}
