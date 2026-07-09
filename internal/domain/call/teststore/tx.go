package teststore

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/domain/storage"
)

// WithinTx executes the callback against a serializable snapshot.
func (s *memoryStore) WithinTx(ctx context.Context, fn func(call.Store) error) error {
	if ctx == nil {
		return call.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if fn == nil {
		return call.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := s.cloneLocked()
	err := fn(&tx)
	if err == nil {
		s.replaceLocked(&tx)
		return nil
	}
	if storage.IsCommit(err) {
		s.replaceLocked(&tx)
		return storage.UnwrapCommit(err)
	}

	return err
}
