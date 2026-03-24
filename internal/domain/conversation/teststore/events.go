package teststore

import (
	"context"
	"slices"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func (s *memoryStore) SaveSyncState(ctx context.Context, state conversation.SyncState) (conversation.SyncState, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.SyncState{}, err
	}
	state.DeviceID = strings.TrimSpace(state.DeviceID)
	state.AccountID = strings.TrimSpace(state.AccountID)
	if state.DeviceID == "" {
		return conversation.SyncState{}, conversation.ErrInvalidInput
	}

	s.syncStatesByDevice[state.DeviceID] = cloneSyncState(state)
	return cloneSyncState(state), nil
}

func (s *memoryStore) SyncStateByDevice(ctx context.Context, deviceID string) (conversation.SyncState, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.SyncState{}, err
	}
	deviceID = strings.TrimSpace(deviceID)
	state, ok := s.syncStatesByDevice[deviceID]
	if !ok {
		return conversation.SyncState{}, conversation.ErrNotFound
	}

	return cloneSyncState(state), nil
}

func (s *memoryStore) SaveEvent(ctx context.Context, event conversation.EventEnvelope) (conversation.EventEnvelope, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.EventEnvelope{}, err
	}
	event.EventID = strings.TrimSpace(event.EventID)
	event.ConversationID = strings.TrimSpace(event.ConversationID)
	event.ActorAccountID = strings.TrimSpace(event.ActorAccountID)
	if event.EventID == "" || event.ConversationID == "" || event.ActorAccountID == "" {
		return conversation.EventEnvelope{}, conversation.ErrInvalidInput
	}

	s.nextSequence++
	event.Sequence = s.nextSequence
	s.eventsByID[event.EventID] = cloneEvent(event)
	s.eventOrder = append(s.eventOrder, event.EventID)
	return cloneEvent(event), nil
}

func (s *memoryStore) EventsAfterSequence(ctx context.Context, fromSequence uint64, limit int, conversationIDs []string) ([]conversation.EventEnvelope, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}

	filter := make(map[string]struct{}, len(conversationIDs))
	for _, conversationID := range conversationIDs {
		conversationID = strings.TrimSpace(conversationID)
		if conversationID == "" {
			continue
		}
		filter[conversationID] = struct{}{}
	}

	events := make([]conversation.EventEnvelope, 0)
	for _, eventID := range s.eventOrder {
		event, ok := s.eventsByID[eventID]
		if !ok || event.Sequence <= fromSequence {
			continue
		}
		if len(filter) > 0 {
			if _, ok := filter[event.ConversationID]; !ok {
				continue
			}
		}
		events = append(events, cloneEvent(event))
		if limit > 0 && len(events) >= limit {
			break
		}
	}

	return slices.Clone(events), nil
}
