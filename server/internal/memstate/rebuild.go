package memstate

import "context"

func (s *Store) Rebuild(ctx context.Context) error {
	snap, live, err := s.load(ctx)
	if err != nil {
		return err
	}
	s.snapshot.Store(snap)
	s.Prune(live.spend, live.providers)
	return nil
}
