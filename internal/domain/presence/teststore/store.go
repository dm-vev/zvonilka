package teststore

import (
	"context"
	"strings"
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/presence"
)

// NewMemoryStore builds a concurrency-safe in-memory presence store for tests.
func NewMemoryStore() presence.Store {
	return &memoryStore{
		presences: make(map[string]presence.Presence),
	}
}

type memoryStore struct {
	mu        sync.RWMutex
	presences map[string]presence.Presence
}

func (s *memoryStore) WithinTx(_ context.Context, fn func(presence.Store) error) error {
	if fn == nil {
		return presence.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := &txStore{
		presences: clonePresences(s.presences),
	}
	if err := fn(tx); err != nil {
		return err
	}

	s.presences = clonePresences(tx.presences)
	return nil
}

func (s *memoryStore) SavePresence(_ context.Context, next presence.Presence) (presence.Presence, error) {
	next, err := presence.Normalize(next)
	if err != nil {
		return presence.Presence{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.presences[next.AccountID] = clonePresence(next)
	return clonePresence(next), nil
}

func (s *memoryStore) PresenceByAccountID(_ context.Context, accountID string) (presence.Presence, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return presence.Presence{}, presence.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.presences[accountID]
	if !ok {
		return presence.Presence{}, presence.ErrNotFound
	}

	return clonePresence(value), nil
}

func (s *memoryStore) PresencesByAccountIDs(_ context.Context, accountIDs []string) (map[string]presence.Presence, error) {
	ids := presence.NormalizeIDs(accountIDs)
	if len(ids) == 0 {
		return make(map[string]presence.Presence), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]presence.Presence, len(ids))
	for _, accountID := range ids {
		value, ok := s.presences[accountID]
		if !ok {
			continue
		}
		result[accountID] = clonePresence(value)
	}

	return result, nil
}

type txStore struct {
	presences map[string]presence.Presence
}

func (s *txStore) WithinTx(_ context.Context, fn func(presence.Store) error) error {
	if fn == nil {
		return presence.ErrInvalidInput
	}

	child := &txStore{
		presences: clonePresences(s.presences),
	}
	if err := fn(child); err != nil {
		return err
	}

	s.presences = clonePresences(child.presences)
	return nil
}

func (s *txStore) SavePresence(_ context.Context, next presence.Presence) (presence.Presence, error) {
	next, err := presence.Normalize(next)
	if err != nil {
		return presence.Presence{}, err
	}

	s.presences[next.AccountID] = clonePresence(next)
	return clonePresence(next), nil
}

func (s *txStore) PresenceByAccountID(_ context.Context, accountID string) (presence.Presence, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return presence.Presence{}, presence.ErrInvalidInput
	}

	value, ok := s.presences[accountID]
	if !ok {
		return presence.Presence{}, presence.ErrNotFound
	}

	return clonePresence(value), nil
}

func (s *txStore) PresencesByAccountIDs(_ context.Context, accountIDs []string) (map[string]presence.Presence, error) {
	ids := presence.NormalizeIDs(accountIDs)
	if len(ids) == 0 {
		return make(map[string]presence.Presence), nil
	}

	result := make(map[string]presence.Presence, len(ids))
	for _, accountID := range ids {
		value, ok := s.presences[accountID]
		if !ok {
			continue
		}
		result[accountID] = clonePresence(value)
	}

	return result, nil
}
