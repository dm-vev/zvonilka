package teststore

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

// NewMemoryStore builds a concurrency-safe in-memory notification store for tests.
func NewMemoryStore() notification.Store {
	return &memoryStore{
		preferencesByAccountID: make(map[string]notification.Preference),
		overridesByKey:         make(map[string]notification.ConversationOverride),
		pushTokensByID:         make(map[string]notification.PushToken),
		pushTokenIDsByAccount:  make(map[string]map[string]struct{}),
		pushTokenIDsByDevice:   make(map[string]string),
		pushTokenIDsByToken:    make(map[string]string),
		deliveriesByID:         make(map[string]notification.Delivery),
		deliveryIDsByDedup:     make(map[string]string),
		workerCursorsByName:    make(map[string]notification.WorkerCursor),
	}
}

type memoryStore struct {
	mu sync.RWMutex

	preferencesByAccountID map[string]notification.Preference
	overridesByKey         map[string]notification.ConversationOverride
	pushTokensByID         map[string]notification.PushToken
	pushTokenIDsByAccount  map[string]map[string]struct{}
	pushTokenIDsByDevice   map[string]string
	pushTokenIDsByToken    map[string]string
	deliveriesByID         map[string]notification.Delivery
	deliveryIDsByDedup     map[string]string
	workerCursorsByName    map[string]notification.WorkerCursor
}

type txStore struct {
	*memoryStore
}

func (s *memoryStore) WithinTx(_ context.Context, fn func(notification.Store) error) error {
	if s == nil {
		return notification.ErrInvalidInput
	}
	if fn == nil {
		return notification.ErrInvalidInput
	}

	s.mu.RLock()
	snapshot := s.cloneLocked()
	s.mu.RUnlock()

	tx := &txStore{memoryStore: snapshot}
	if err := fn(tx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.replaceLocked(snapshot)

	return nil
}

func (s *memoryStore) SavePreference(_ context.Context, preference notification.Preference) (notification.Preference, error) {
	preference, err := notification.NormalizePreference(preference, time.Now().UTC())
	if err != nil {
		return notification.Preference{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.preferencesByAccountID[preference.AccountID] = clonePreference(preference)
	return clonePreference(preference), nil
}

func (s *memoryStore) PreferenceByAccountID(_ context.Context, accountID string) (notification.Preference, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	preference, ok := s.preferencesByAccountID[accountID]
	if !ok {
		return notification.Preference{}, notification.ErrNotFound
	}

	return clonePreference(preference), nil
}

func (s *memoryStore) SaveOverride(_ context.Context, override notification.ConversationOverride) (notification.ConversationOverride, error) {
	override, err := notification.NormalizeConversationOverride(override, time.Now().UTC())
	if err != nil {
		return notification.ConversationOverride{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.overridesByKey[overrideKey(override.ConversationID, override.AccountID)] = cloneOverride(override)
	return cloneOverride(override), nil
}

func (s *memoryStore) OverrideByConversationAndAccount(_ context.Context, conversationID string, accountID string) (notification.ConversationOverride, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	override, ok := s.overridesByKey[overrideKey(conversationID, accountID)]
	if !ok {
		return notification.ConversationOverride{}, notification.ErrNotFound
	}

	return cloneOverride(override), nil
}

func (s *memoryStore) SavePushToken(_ context.Context, token notification.PushToken) (notification.PushToken, error) {
	token, err := notification.NormalizePushToken(token, time.Now().UTC())
	if err != nil {
		return notification.PushToken{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.pushTokenIDsByDevice[token.DeviceID]; ok && existingID != token.ID {
		existing := s.pushTokensByID[existingID]
		if existing.AccountID != token.AccountID {
			return notification.PushToken{}, notification.ErrConflict
		}
		s.deletePushTokenLocked(existing)
	}

	if existingID, ok := s.pushTokenIDsByToken[token.Token]; ok && existingID != token.ID {
		existing := s.pushTokensByID[existingID]
		if existing.AccountID != token.AccountID || existing.DeviceID != token.DeviceID {
			return notification.PushToken{}, notification.ErrConflict
		}
		s.deletePushTokenLocked(existing)
	}

	if previous, ok := s.pushTokensByID[token.ID]; ok {
		s.deletePushTokenLocked(previous)
	}

	s.pushTokensByID[token.ID] = clonePushToken(token)
	s.ensurePushAccountIndexLocked(token.AccountID)[token.ID] = struct{}{}
	s.pushTokenIDsByDevice[token.DeviceID] = token.ID
	s.pushTokenIDsByToken[token.Token] = token.ID

	return clonePushToken(token), nil
}

func (s *memoryStore) PushTokenByID(_ context.Context, tokenID string) (notification.PushToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	token, ok := s.pushTokensByID[tokenID]
	if !ok {
		return notification.PushToken{}, notification.ErrNotFound
	}

	return clonePushToken(token), nil
}

func (s *memoryStore) PushTokensByAccountID(_ context.Context, accountID string) ([]notification.PushToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tokenIDs := s.pushTokenIDsByAccount[accountID]
	if len(tokenIDs) == 0 {
		return nil, nil
	}

	tokens := make([]notification.PushToken, 0, len(tokenIDs))
	for tokenID := range tokenIDs {
		token, ok := s.pushTokensByID[tokenID]
		if !ok || !token.Enabled || !token.RevokedAt.IsZero() {
			continue
		}
		tokens = append(tokens, clonePushToken(token))
	}

	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].CreatedAt.Equal(tokens[j].CreatedAt) {
			return tokens[i].ID < tokens[j].ID
		}
		return tokens[i].CreatedAt.Before(tokens[j].CreatedAt)
	})

	return tokens, nil
}

func (s *memoryStore) DeletePushToken(_ context.Context, tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, ok := s.pushTokensByID[tokenID]
	if !ok {
		return notification.ErrNotFound
	}

	s.deletePushTokenLocked(token)
	return nil
}

func (s *memoryStore) SaveDelivery(_ context.Context, delivery notification.Delivery) (notification.Delivery, error) {
	delivery, err := notification.NormalizeDelivery(delivery, time.Now().UTC())
	if err != nil {
		return notification.Delivery{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.deliveryIDsByDedup[delivery.DedupKey]; ok && existingID != "" {
		if existing, ok := s.deliveriesByID[existingID]; ok {
			if delivery.Attempts <= existing.Attempts {
				return cloneDelivery(existing), nil
			}
			delivery.ID = existing.ID
		}
	} else if previous, ok := s.deliveriesByID[delivery.ID]; ok {
		delete(s.deliveryIDsByDedup, previous.DedupKey)
	}

	s.deliveriesByID[delivery.ID] = cloneDelivery(delivery)
	s.deliveryIDsByDedup[delivery.DedupKey] = delivery.ID

	return cloneDelivery(delivery), nil
}

func (s *memoryStore) DeliveryByID(_ context.Context, deliveryID string) (notification.Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	delivery, ok := s.deliveriesByID[deliveryID]
	if !ok {
		return notification.Delivery{}, notification.ErrNotFound
	}

	return cloneDelivery(delivery), nil
}

func (s *memoryStore) DeliveriesDue(_ context.Context, before time.Time, limit int) ([]notification.Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		return nil, nil
	}

	deliveries := make([]notification.Delivery, 0)
	for _, delivery := range s.deliveriesByID {
		if delivery.State != notification.DeliveryStateQueued {
			continue
		}
		if delivery.NextAttemptAt.After(before) {
			continue
		}
		if !delivery.LeaseExpiresAt.IsZero() && delivery.LeaseExpiresAt.After(before) {
			continue
		}
		deliveries = append(deliveries, cloneDelivery(delivery))
	}

	sort.Slice(deliveries, func(i, j int) bool {
		if deliveries[i].Priority == deliveries[j].Priority {
			if deliveries[i].NextAttemptAt.Equal(deliveries[j].NextAttemptAt) {
				if deliveries[i].CreatedAt.Equal(deliveries[j].CreatedAt) {
					return deliveries[i].ID < deliveries[j].ID
				}
				return deliveries[i].CreatedAt.Before(deliveries[j].CreatedAt)
			}
			return deliveries[i].NextAttemptAt.Before(deliveries[j].NextAttemptAt)
		}
		return deliveries[i].Priority > deliveries[j].Priority
	})
	if len(deliveries) > limit {
		deliveries = deliveries[:limit]
	}

	return deliveries, nil
}

func (s *memoryStore) ClaimDeliveries(_ context.Context, params notification.ClaimDeliveriesParams) ([]notification.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if params.Limit <= 0 {
		return nil, nil
	}

	claimTime := params.Before.UTC()
	if claimTime.IsZero() {
		claimTime = time.Now().UTC()
	}
	leaseToken := params.LeaseToken
	if leaseToken == "" || params.LeaseDuration <= 0 {
		return nil, notification.ErrInvalidInput
	}

	deliveries := make([]notification.Delivery, 0)
	for _, delivery := range s.deliveriesByID {
		if delivery.State != notification.DeliveryStateQueued {
			continue
		}
		if delivery.NextAttemptAt.After(claimTime) {
			continue
		}
		if !delivery.LeaseExpiresAt.IsZero() && delivery.LeaseExpiresAt.After(claimTime) {
			continue
		}
		deliveries = append(deliveries, cloneDelivery(delivery))
	}

	sort.Slice(deliveries, func(i, j int) bool {
		if deliveries[i].Priority == deliveries[j].Priority {
			if deliveries[i].NextAttemptAt.Equal(deliveries[j].NextAttemptAt) {
				if deliveries[i].CreatedAt.Equal(deliveries[j].CreatedAt) {
					return deliveries[i].ID < deliveries[j].ID
				}
				return deliveries[i].CreatedAt.Before(deliveries[j].CreatedAt)
			}
			return deliveries[i].NextAttemptAt.Before(deliveries[j].NextAttemptAt)
		}
		return deliveries[i].Priority > deliveries[j].Priority
	})
	if len(deliveries) > params.Limit {
		deliveries = deliveries[:params.Limit]
	}

	claimed := make([]notification.Delivery, 0, len(deliveries))
	for _, delivery := range deliveries {
		stored := s.deliveriesByID[delivery.ID]
		stored.LeaseToken = leaseToken
		stored.LeaseExpiresAt = claimTime.Add(params.LeaseDuration)
		stored.UpdatedAt = claimTime
		s.deliveriesByID[stored.ID] = cloneDelivery(stored)
		claimed = append(claimed, cloneDelivery(stored))
	}

	return claimed, nil
}

func (s *memoryStore) MarkDeliveryDelivered(_ context.Context, params notification.MarkDeliveryDeliveredParams) (notification.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := params.DeliveredAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	delivery, err := s.lockedDeliveryForLeaseAt(params.DeliveryID, params.LeaseToken, now)
	if err != nil {
		return notification.Delivery{}, err
	}
	delivery.Attempts++
	delivery.State = notification.DeliveryStateDelivered
	delivery.LastAttemptAt = now
	delivery.LastError = ""
	delivery.NextAttemptAt = now
	delivery.LeaseToken = ""
	delivery.LeaseExpiresAt = time.Time{}
	delivery.UpdatedAt = now
	s.deliveriesByID[delivery.ID] = cloneDelivery(delivery)

	return cloneDelivery(delivery), nil
}

func (s *memoryStore) FailDelivery(_ context.Context, params notification.FailDeliveryParams) (notification.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := params.FailedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	delivery, err := s.lockedDeliveryForLeaseAt(params.DeliveryID, params.LeaseToken, now)
	if err != nil {
		return notification.Delivery{}, err
	}
	delivery.Attempts++
	delivery.State = notification.DeliveryStateFailed
	delivery.LastAttemptAt = now
	delivery.LastError = params.LastError
	delivery.NextAttemptAt = now
	delivery.LeaseToken = ""
	delivery.LeaseExpiresAt = time.Time{}
	delivery.UpdatedAt = now
	s.deliveriesByID[delivery.ID] = cloneDelivery(delivery)

	return cloneDelivery(delivery), nil
}

func (s *memoryStore) RetryDelivery(_ context.Context, params notification.RetryDeliveryParams) (notification.Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delivery, ok := s.deliveriesByID[params.DeliveryID]
	if !ok {
		return notification.Delivery{}, notification.ErrNotFound
	}
	if params.LeaseToken != "" {
		var err error
		delivery, err = s.lockedDeliveryForLeaseAt(params.DeliveryID, params.LeaseToken, params.AttemptedAt)
		if err != nil {
			return notification.Delivery{}, err
		}
	}
	switch delivery.State {
	case notification.DeliveryStateDelivered, notification.DeliveryStateSuppressed:
		return notification.Delivery{}, notification.ErrConflict
	}

	now := params.AttemptedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	delivery.Attempts++
	delivery.LastError = params.LastError
	delivery.LastAttemptAt = now
	delivery.NextAttemptAt = params.RetryAt
	if delivery.NextAttemptAt.IsZero() {
		delivery.NextAttemptAt = delivery.LastAttemptAt
	}
	if params.MaxAttempts > 0 && delivery.Attempts >= params.MaxAttempts {
		delivery.State = notification.DeliveryStateFailed
		delivery.NextAttemptAt = now
	} else {
		delivery.State = notification.DeliveryStateQueued
	}
	delivery.LeaseToken = ""
	delivery.LeaseExpiresAt = time.Time{}
	delivery.UpdatedAt = delivery.LastAttemptAt

	s.deliveriesByID[delivery.ID] = cloneDelivery(delivery)
	s.deliveryIDsByDedup[delivery.DedupKey] = delivery.ID

	return cloneDelivery(delivery), nil
}

func (s *memoryStore) SaveWorkerCursor(_ context.Context, cursor notification.WorkerCursor) (notification.WorkerCursor, error) {
	cursor, err := notification.NormalizeWorkerCursor(cursor, time.Now().UTC())
	if err != nil {
		return notification.WorkerCursor{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.workerCursorsByName[cursor.Name]; ok && cursor.LastSequence <= existing.LastSequence {
		return cloneCursor(existing), nil
	}

	s.workerCursorsByName[cursor.Name] = cloneCursor(cursor)
	return cloneCursor(cursor), nil
}

func (s *memoryStore) WorkerCursorByName(_ context.Context, name string) (notification.WorkerCursor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cursor, ok := s.workerCursorsByName[name]
	if !ok {
		return notification.WorkerCursor{}, notification.ErrNotFound
	}

	return cloneCursor(cursor), nil
}

func (s *memoryStore) cloneLocked() *memoryStore {
	clone := NewMemoryStore().(*memoryStore)

	for key, value := range s.preferencesByAccountID {
		clone.preferencesByAccountID[key] = clonePreference(value)
	}
	for key, value := range s.overridesByKey {
		clone.overridesByKey[key] = cloneOverride(value)
	}
	for key, value := range s.pushTokensByID {
		clone.pushTokensByID[key] = clonePushToken(value)
	}
	for key, value := range s.deliveriesByID {
		clone.deliveriesByID[key] = cloneDelivery(value)
	}
	for key, value := range s.workerCursorsByName {
		clone.workerCursorsByName[key] = cloneCursor(value)
	}
	for accountID, tokenIDs := range s.pushTokenIDsByAccount {
		clone.pushTokenIDsByAccount[accountID] = make(map[string]struct{}, len(tokenIDs))
		for tokenID := range tokenIDs {
			clone.pushTokenIDsByAccount[accountID][tokenID] = struct{}{}
		}
	}
	for key, value := range s.pushTokenIDsByDevice {
		clone.pushTokenIDsByDevice[key] = value
	}
	for key, value := range s.pushTokenIDsByToken {
		clone.pushTokenIDsByToken[key] = value
	}
	for key, value := range s.deliveryIDsByDedup {
		clone.deliveryIDsByDedup[key] = value
	}

	return clone
}

func (s *memoryStore) replaceLocked(other *memoryStore) {
	s.preferencesByAccountID = other.preferencesByAccountID
	s.overridesByKey = other.overridesByKey
	s.pushTokensByID = other.pushTokensByID
	s.pushTokenIDsByAccount = other.pushTokenIDsByAccount
	s.pushTokenIDsByDevice = other.pushTokenIDsByDevice
	s.pushTokenIDsByToken = other.pushTokenIDsByToken
	s.deliveriesByID = other.deliveriesByID
	s.deliveryIDsByDedup = other.deliveryIDsByDedup
	s.workerCursorsByName = other.workerCursorsByName
}

func (s *memoryStore) ensurePushAccountIndexLocked(accountID string) map[string]struct{} {
	index, ok := s.pushTokenIDsByAccount[accountID]
	if !ok || index == nil {
		index = make(map[string]struct{})
		s.pushTokenIDsByAccount[accountID] = index
	}

	return index
}

func (s *memoryStore) deletePushTokenLocked(token notification.PushToken) {
	delete(s.pushTokensByID, token.ID)
	delete(s.pushTokenIDsByDevice, token.DeviceID)
	delete(s.pushTokenIDsByToken, token.Token)
	if tokenIndex, ok := s.pushTokenIDsByAccount[token.AccountID]; ok {
		delete(tokenIndex, token.ID)
		if len(tokenIndex) == 0 {
			delete(s.pushTokenIDsByAccount, token.AccountID)
		}
	}
}

func (s *memoryStore) lockedDeliveryForLease(deliveryID string, leaseToken string) (notification.Delivery, error) {
	return s.lockedDeliveryForLeaseAt(deliveryID, leaseToken, time.Time{})
}

func (s *memoryStore) lockedDeliveryForLeaseAt(
	deliveryID string,
	leaseToken string,
	at time.Time,
) (notification.Delivery, error) {
	delivery, ok := s.deliveriesByID[deliveryID]
	if !ok {
		return notification.Delivery{}, notification.ErrNotFound
	}
	if delivery.State != notification.DeliveryStateQueued {
		return notification.Delivery{}, notification.ErrConflict
	}
	if leaseToken != "" {
		if delivery.LeaseToken != leaseToken {
			return notification.Delivery{}, notification.ErrConflict
		}
		now := at.UTC()
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if delivery.LeaseExpiresAt.IsZero() || delivery.LeaseExpiresAt.Before(now) {
			return notification.Delivery{}, notification.ErrConflict
		}
	}

	return cloneDelivery(delivery), nil
}

func overrideKey(conversationID string, accountID string) string {
	return conversationID + ":" + accountID
}

func clonePreference(value notification.Preference) notification.Preference { return value }
func cloneOverride(value notification.ConversationOverride) notification.ConversationOverride {
	return value
}
func clonePushToken(value notification.PushToken) notification.PushToken    { return value }
func cloneDelivery(value notification.Delivery) notification.Delivery       { return value }
func cloneCursor(value notification.WorkerCursor) notification.WorkerCursor { return value }

var _ notification.Store = (*memoryStore)(nil)
var _ notification.Store = (*txStore)(nil)
