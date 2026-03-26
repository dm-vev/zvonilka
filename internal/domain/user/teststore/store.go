package teststore

import (
	"context"
	"sync"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

// NewMemoryStore constructs a concurrency-safe in-memory user store.
func NewMemoryStore() domainuser.Store {
	return &memoryStore{
		privacies: make(map[string]domainuser.Privacy),
		contacts:  make(map[string]domainuser.Contact),
		blocks:    make(map[string]domainuser.BlockEntry),
	}
}

type memoryStore struct {
	mu        sync.RWMutex
	privacies map[string]domainuser.Privacy
	contacts  map[string]domainuser.Contact
	blocks    map[string]domainuser.BlockEntry
}

func (s *memoryStore) WithinTx(ctx context.Context, fn func(domainuser.Store) error) error {
	if ctx == nil {
		return domainuser.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if fn == nil {
		return domainuser.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := &memoryStore{
		privacies: clonePrivacies(s.privacies),
		contacts:  cloneContacts(s.contacts),
		blocks:    cloneBlocks(s.blocks),
	}
	if err := fn(tx); err != nil {
		return err
	}

	s.privacies = clonePrivacies(tx.privacies)
	s.contacts = cloneContacts(tx.contacts)
	s.blocks = cloneBlocks(tx.blocks)
	return nil
}

func clonePrivacies(src map[string]domainuser.Privacy) map[string]domainuser.Privacy {
	dst := make(map[string]domainuser.Privacy, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneContacts(src map[string]domainuser.Contact) map[string]domainuser.Contact {
	dst := make(map[string]domainuser.Contact, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func cloneBlocks(src map[string]domainuser.BlockEntry) map[string]domainuser.BlockEntry {
	dst := make(map[string]domainuser.BlockEntry, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func contactKey(accountID string, userID string) string {
	return accountID + ":" + userID
}

var _ domainuser.Store = (*memoryStore)(nil)
