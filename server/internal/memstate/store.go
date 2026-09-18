package memstate

import (
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool      *pgxpool.Pool
	masterKey string

	snapshot atomic.Pointer[Snapshot]

	providerHourlyCalls sync.Map
	dailySpendNanos     sync.Map

	dailySpendSynced     sync.Map
	userModelHourlyCalls sync.Map

	providerLimiters sync.Map

	killSwitch atomic.Bool
}

func New(pool *pgxpool.Pool, masterKey string) *Store {
	s := &Store{pool: pool, masterKey: masterKey}
	s.snapshot.Store(&Snapshot{KeyIndex: map[[32]byte]ResolvedKey{}})
	return s
}

func (s *Store) Load() *Snapshot {
	return s.snapshot.Load()
}

func (s *Store) KillSwitchTripped() bool {
	return s.killSwitch.Load()
}

func (s *Store) SetKillSwitch(tripped bool) {
	s.killSwitch.Store(tripped)
}
