package teststore

import (
	"context"
	"sync"

	"github.com/dm-vev/zvonilka/internal/domain/e2ee"
)

func NewMemoryStore() e2ee.Store {
	return &memoryStore{
		signed:   make(map[string]e2ee.SignedPreKey),
		oneTime:  make(map[string][]e2ee.OneTimePreKey),
		sessions: make(map[string]e2ee.DirectSession),
	}
}

type memoryStore struct {
	mu      sync.RWMutex
	signed   map[string]e2ee.SignedPreKey
	oneTime  map[string][]e2ee.OneTimePreKey
	sessions map[string]e2ee.DirectSession
}

func (s *memoryStore) WithinTx(_ context.Context, fn func(e2ee.Store) error) error {
	if fn == nil {
		return e2ee.ErrInvalidInput
	}
	return fn(s)
}

func (s *memoryStore) SaveSignedPreKey(_ context.Context, accountID string, deviceID string, value e2ee.SignedPreKey) (e2ee.SignedPreKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signed[key(accountID, deviceID)] = value
	return value, nil
}

func (s *memoryStore) SignedPreKeyByDevice(_ context.Context, accountID string, deviceID string) (e2ee.SignedPreKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.signed[key(accountID, deviceID)]
	if !ok {
		return e2ee.SignedPreKey{}, e2ee.ErrNotFound
	}
	return value, nil
}

func (s *memoryStore) DeleteOneTimePreKeysByDevice(_ context.Context, accountID string, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.oneTime, key(accountID, deviceID))
	return nil
}

func (s *memoryStore) SaveOneTimePreKeys(_ context.Context, accountID string, deviceID string, values []e2ee.OneTimePreKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row := append([]e2ee.OneTimePreKey(nil), s.oneTime[key(accountID, deviceID)]...)
	row = append(row, values...)
	s.oneTime[key(accountID, deviceID)] = row
	return nil
}

func (s *memoryStore) ClaimOneTimePreKey(
	_ context.Context,
	accountID string,
	deviceID string,
	claimedByAccountID string,
	claimedByDeviceID string,
) (e2ee.OneTimePreKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values := s.oneTime[key(accountID, deviceID)]
	for i := range values {
		if !values[i].ClaimedAt.IsZero() {
			continue
		}
		values[i].ClaimedByAccountID = claimedByAccountID
		values[i].ClaimedByDeviceID = claimedByDeviceID
		values[i].ClaimedAt = values[i].Key.CreatedAt
		if values[i].ClaimedAt.IsZero() {
			values[i].ClaimedAt = values[i].Key.RotatedAt
		}
		s.oneTime[key(accountID, deviceID)] = values
		return values[i], nil
	}
	return e2ee.OneTimePreKey{}, e2ee.ErrNotFound
}

func (s *memoryStore) CountAvailableOneTimePreKeys(_ context.Context, accountID string, deviceID string) (uint32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var total uint32
	for _, value := range s.oneTime[key(accountID, deviceID)] {
		if value.ClaimedAt.IsZero() {
			total++
		}
	}
	return total, nil
}

func (s *memoryStore) SaveDirectSession(_ context.Context, value e2ee.DirectSession) (e2ee.DirectSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[value.ID] = cloneDirectSession(value)
	return cloneDirectSession(value), nil
}

func (s *memoryStore) DirectSessionByID(_ context.Context, sessionID string) (e2ee.DirectSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.sessions[sessionID]
	if !ok {
		return e2ee.DirectSession{}, e2ee.ErrNotFound
	}
	return cloneDirectSession(value), nil
}

func (s *memoryStore) DirectSessionsByRecipientDevice(_ context.Context, accountID string, deviceID string) ([]e2ee.DirectSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]e2ee.DirectSession, 0)
	for _, value := range s.sessions {
		if value.RecipientAccountID != accountID || value.RecipientDeviceID != deviceID {
			continue
		}
		result = append(result, cloneDirectSession(value))
	}
	return result, nil
}

func key(accountID string, deviceID string) string {
	return accountID + "|" + deviceID
}

func cloneDirectSession(value e2ee.DirectSession) e2ee.DirectSession {
	value.InitiatorEphemeral = clonePublicKey(value.InitiatorEphemeral)
	value.IdentityKey = clonePublicKey(value.IdentityKey)
	value.SignedPreKey = e2ee.SignedPreKey{
		Key:       clonePublicKey(value.SignedPreKey.Key),
		Signature: append([]byte(nil), value.SignedPreKey.Signature...),
	}
	value.OneTimePreKey = e2ee.OneTimePreKey{
		Key:                clonePublicKey(value.OneTimePreKey.Key),
		ClaimedAt:          value.OneTimePreKey.ClaimedAt,
		ClaimedByAccountID: value.OneTimePreKey.ClaimedByAccountID,
		ClaimedByDeviceID:  value.OneTimePreKey.ClaimedByDeviceID,
	}
	value.Bootstrap = e2ee.BootstrapPayload{
		Algorithm:  value.Bootstrap.Algorithm,
		Nonce:      append([]byte(nil), value.Bootstrap.Nonce...),
		Ciphertext: append([]byte(nil), value.Bootstrap.Ciphertext...),
	}
	if len(value.Bootstrap.Metadata) > 0 {
		value.Bootstrap.Metadata = make(map[string]string, len(value.Bootstrap.Metadata))
		for key, item := range value.Bootstrap.Metadata {
			value.Bootstrap.Metadata[key] = item
		}
	}
	return value
}

func clonePublicKey(value e2ee.PublicKey) e2ee.PublicKey {
	value.PublicKey = append([]byte(nil), value.PublicKey...)
	return value
}

var _ e2ee.Store = (*memoryStore)(nil)
