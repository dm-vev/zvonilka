package teststore

import (
	"context"
	"sort"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func (s *memoryStore) SaveBlock(_ context.Context, block domainuser.BlockEntry) (domainuser.BlockEntry, error) {
	if block.OwnerAccountID == "" || block.BlockedAccountID == "" || block.OwnerAccountID == block.BlockedAccountID {
		return domainuser.BlockEntry{}, domainuser.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.blocks[contactKey(block.OwnerAccountID, block.BlockedAccountID)] = block
	return block, nil
}

func (s *memoryStore) BlockByAccountAndUserID(
	_ context.Context,
	accountID string,
	userID string,
) (domainuser.BlockEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, ok := s.blocks[contactKey(accountID, userID)]
	if !ok {
		return domainuser.BlockEntry{}, domainuser.ErrNotFound
	}

	return block, nil
}

func (s *memoryStore) ListBlocksByAccountID(_ context.Context, accountID string) ([]domainuser.BlockEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	blocks := make([]domainuser.BlockEntry, 0)
	for _, block := range s.blocks {
		if block.OwnerAccountID != accountID {
			continue
		}
		blocks = append(blocks, block)
	}
	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].BlockedAt.Equal(blocks[j].BlockedAt) {
			return blocks[i].BlockedAccountID < blocks[j].BlockedAccountID
		}
		return blocks[i].BlockedAt.Before(blocks[j].BlockedAt)
	})
	return blocks, nil
}

func (s *memoryStore) DeleteBlock(_ context.Context, accountID string, userID string) error {
	key := contactKey(accountID, userID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.blocks[key]; !ok {
		return domainuser.ErrNotFound
	}
	delete(s.blocks, key)
	return nil
}
