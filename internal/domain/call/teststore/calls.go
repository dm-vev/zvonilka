package teststore

import (
	"context"
	"slices"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

func (s *memoryStore) SaveCall(_ context.Context, value call.Call) (call.Call, error) {
	saved, err := normalizeCall(value)
	if err != nil {
		return call.Call{}, err
	}

	s.callsByID[saved.ID] = callClone(saved)
	return callClone(saved), nil
}

func (s *memoryStore) CallByID(_ context.Context, callID string) (call.Call, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return call.Call{}, call.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.callsByID[callID]
	if !ok {
		return call.Call{}, call.ErrNotFound
	}

	return callClone(value), nil
}

func (s *memoryStore) ActiveCallByConversation(_ context.Context, conversationID string) (call.Call, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return call.Call{}, call.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, value := range s.callsByID {
		if value.ConversationID != conversationID {
			continue
		}
		if value.State != call.StateRinging && value.State != call.StateActive {
			continue
		}
		return callClone(value), nil
	}

	return call.Call{}, call.ErrNotFound
}

func (s *memoryStore) CallsByConversation(_ context.Context, conversationID string, includeEnded bool) ([]call.Call, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, call.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]call.Call, 0)
	for _, value := range s.callsByID {
		if value.ConversationID != conversationID {
			continue
		}
		if !includeEnded && value.State == call.StateEnded {
			continue
		}
		result = append(result, callClone(value))
	}

	slices.SortFunc(result, func(left call.Call, right call.Call) int {
		if left.StartedAt.Before(right.StartedAt) {
			return -1
		}
		if left.StartedAt.After(right.StartedAt) {
			return 1
		}
		if left.ID < right.ID {
			return -1
		}
		if left.ID > right.ID {
			return 1
		}
		return 0
	})

	return result, nil
}

func normalizeCall(value call.Call) (call.Call, error) {
	value.ID = strings.TrimSpace(value.ID)
	value.ConversationID = strings.TrimSpace(value.ConversationID)
	value.InitiatorAccountID = strings.TrimSpace(value.InitiatorAccountID)
	value.ActiveSessionID = strings.TrimSpace(value.ActiveSessionID)
	if value.ID == "" || value.ConversationID == "" || value.InitiatorAccountID == "" {
		return call.Call{}, call.ErrInvalidInput
	}
	if value.State == call.StateUnspecified {
		return call.Call{}, call.ErrInvalidInput
	}

	value.Invites = nil
	value.Participants = nil
	return value, nil
}
