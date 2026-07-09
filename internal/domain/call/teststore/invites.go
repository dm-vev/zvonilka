package teststore

import (
	"context"
	"slices"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

func (s *memoryStore) SaveInvite(_ context.Context, value call.Invite) (call.Invite, error) {
	value.CallID = strings.TrimSpace(value.CallID)
	value.AccountID = strings.TrimSpace(value.AccountID)
	if value.CallID == "" || value.AccountID == "" || value.State == call.InviteStateUnspecified {
		return call.Invite{}, call.ErrInvalidInput
	}

	s.invitesByKey[inviteKey(value.CallID, value.AccountID)] = value
	return value, nil
}

func (s *memoryStore) InviteByCallAndAccount(_ context.Context, callID string, accountID string) (call.Invite, error) {
	callID = strings.TrimSpace(callID)
	accountID = strings.TrimSpace(accountID)
	if callID == "" || accountID == "" {
		return call.Invite{}, call.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	value, ok := s.invitesByKey[inviteKey(callID, accountID)]
	if !ok {
		return call.Invite{}, call.ErrNotFound
	}
	return value, nil
}

func (s *memoryStore) InvitesByCall(_ context.Context, callID string) ([]call.Invite, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, call.ErrInvalidInput
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]call.Invite, 0)
	for _, value := range s.invitesByKey {
		if value.CallID == callID {
			result = append(result, value)
		}
	}
	slices.SortFunc(result, func(left call.Invite, right call.Invite) int {
		if left.AccountID < right.AccountID {
			return -1
		}
		if left.AccountID > right.AccountID {
			return 1
		}
		return 0
	})
	return result, nil
}
