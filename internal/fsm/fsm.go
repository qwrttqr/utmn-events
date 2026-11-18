package fsm

type StateID string

const Idle StateID = "idle"

type session struct {
	state StateID
	draft any
}

type FSM struct {
	sessions *ShardedInMemoryStore[int64, session]
}

func NewInMemoryFSM() *FSM {
	hash := func(userID int64) uint32 {
		value := uint64(userID)
		value ^= value >> 33
		value *= 0xff51afd7ed558ccd
		value ^= value >> 33
		return uint32(value ^ (value >> 32))
	}
	return &FSM{sessions: NewCache[int64, session](32, hash)}
}

func (f *FSM) Replace(userID int64, state StateID, draft any) {
	f.sessions.Set(userID, session{state: state, draft: draft})
}

func (f *FSM) CurrentState(userID int64) StateID {
	active, ok := f.sessions.Get(userID)
	if !ok {
		return Idle
	}
	return active.state
}

func Load[D any](f *FSM, userID int64) (StateID, D, bool) {
	active, _ := f.sessions.Get(userID)
	draft, ok := active.draft.(D)
	if !ok {
		return Idle, draft, false
	}
	return active.state, draft, true
}

func (f *FSM) Clear(userID int64) {
	f.sessions.Delete(userID)
}
