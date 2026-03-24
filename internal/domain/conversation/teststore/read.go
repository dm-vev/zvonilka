package teststore

import (
	"context"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func (s *memoryStore) SaveReadState(ctx context.Context, state conversation.ReadState) (conversation.ReadState, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ReadState{}, err
	}
	state.ConversationID = strings.TrimSpace(state.ConversationID)
	state.AccountID = strings.TrimSpace(state.AccountID)
	state.DeviceID = strings.TrimSpace(state.DeviceID)
	if state.ConversationID == "" || state.AccountID == "" || state.DeviceID == "" {
		return conversation.ReadState{}, conversation.ErrInvalidInput
	}

	key := readStateKey(state.ConversationID, state.AccountID, state.DeviceID)
	s.readStatesByKey[key] = cloneReadState(state)
	return cloneReadState(state), nil
}

func (s *memoryStore) ReadStateByConversationAndDevice(ctx context.Context, conversationID string, deviceID string) (conversation.ReadState, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.ReadState{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	deviceID = strings.TrimSpace(deviceID)
	for _, state := range s.readStatesByKey {
		if state.ConversationID == conversationID && state.DeviceID == deviceID {
			return cloneReadState(state), nil
		}
	}

	return conversation.ReadState{}, conversation.ErrNotFound
}

func (s *memoryStore) ReadStatesByDevice(ctx context.Context, deviceID string) ([]conversation.ReadState, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, conversation.ErrInvalidInput
	}

	states := make([]conversation.ReadState, 0)
	for _, state := range s.readStatesByKey {
		if state.DeviceID != deviceID {
			continue
		}
		states = append(states, cloneReadState(state))
	}

	return states, nil
}
