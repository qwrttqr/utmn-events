package fsm

// NewInMemoryFSM uses the same sharded cache backend for both storage adapters.
// Each conversation owns its stores, so identical user IDs in different
// conversations cannot overwrite one another.
func NewInMemoryFSM[K comparable, V any](initialState StateID) *FSM[K, V] {
	hash := func(userID int64) uint32 {
		value := uint64(userID)
		value ^= value >> 33
		value *= 0xff51afd7ed558ccd
		value ^= value >> 33
		return uint32(value ^ (value >> 32))
	}
	return NewFSM(initialState,
		NewUserStateInMemoryStorage(32, hash),
		NewUserDataInMemoryStorage[K, V](32, hash))
}
