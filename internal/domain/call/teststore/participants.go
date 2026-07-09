package teststore

import (
	"context"
	"slices"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

func (s *memoryStore) SaveParticipant(_ context.Context, value call.Participant) (call.Participant, error) {
	value.CallID = strings.TrimSpace(value.CallID)
	value.AccountID = strings.TrimSpace(value.AccountID)
	value.DeviceID = strings.TrimSpace(value.DeviceID)
	if value.CallID == "" || value.AccountID == "" || value.DeviceID == "" || value.State == call.ParticipantStateUnspecified {
		return call.Participant{}, call.ErrInvalidInput
	}

	s.participantsByKey[participantKey(value.CallID, value.DeviceID)] = value
	return value, nil
}

func (s *memoryStore) ParticipantByCallAndDevice(_ context.Context, callID string, deviceID string) (call.Participant, error) {
	callID = strings.TrimSpace(callID)
	deviceID = strings.TrimSpace(deviceID)
	if callID == "" || deviceID == "" {
		return call.Participant{}, call.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.participantsByKey[participantKey(callID, deviceID)]
	if !ok {
		return call.Participant{}, call.ErrNotFound
	}
	return value, nil
}

func (s *memoryStore) ParticipantsByCall(_ context.Context, callID string) ([]call.Participant, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, call.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]call.Participant, 0)
	for _, value := range s.participantsByKey {
		if value.CallID == callID {
			result = append(result, value)
		}
	}
	slices.SortFunc(result, func(left call.Participant, right call.Participant) int {
		if left.DeviceID < right.DeviceID {
			return -1
		}
		if left.DeviceID > right.DeviceID {
			return 1
		}
		return 0
	})
	return result, nil
}
