package teststore

import (
	"context"
	"sort"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

func (s *memoryStore) SaveJoinRequest(_ context.Context, joinRequest identity.JoinRequest) (identity.JoinRequest, error) {
	if joinRequest.ID == "" {
		return identity.JoinRequest{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.joinRequestsByID[joinRequest.ID]; !exists &&
		joinRequest.Status == identity.JoinRequestStatusPending &&
		joinRequest.ReviewedAt.IsZero() {
		if s.hasAccountConflictLocked("", joinRequest.Username, joinRequest.Email, joinRequest.Phone, "") {
			return identity.JoinRequest{}, identity.ErrConflict
		}
	}

	s.joinRequestsByID[joinRequest.ID] = joinRequest
	return joinRequest, nil
}

func (s *memoryStore) JoinRequestByID(_ context.Context, joinRequestID string) (identity.JoinRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	joinRequest, ok := s.joinRequestsByID[joinRequestID]
	if !ok {
		return identity.JoinRequest{}, identity.ErrNotFound
	}

	return joinRequest, nil
}

func (s *memoryStore) JoinRequestsByStatus(_ context.Context, status identity.JoinRequestStatus) ([]identity.JoinRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	joinRequests := make([]identity.JoinRequest, 0)
	for _, joinRequest := range s.joinRequestsByID {
		if joinRequest.Status == status {
			joinRequests = append(joinRequests, joinRequest)
		}
	}

	sort.Slice(joinRequests, func(i, j int) bool {
		return joinRequests[i].RequestedAt.Before(joinRequests[j].RequestedAt)
	})

	return joinRequests, nil
}
