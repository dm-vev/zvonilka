package teststore

import (
	"context"
	"sort"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveJoinRequest inserts or updates a join request.
//
// Pending requests are conflict-checked against existing accounts before they are stored so
// the approval flow sees the same atomic uniqueness behavior as account creation.
func (s *memoryStore) SaveJoinRequest(_ context.Context, joinRequest identity.JoinRequest) (identity.JoinRequest, error) {
	if joinRequest.ID == "" {
		return identity.JoinRequest{}, identity.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous, exists := s.joinRequestsByID[joinRequest.ID]

	if joinRequest.Status == identity.JoinRequestStatusPending && joinRequest.ReviewedAt.IsZero() {
		if s.hasAccountConflictLocked("", joinRequest.Username, joinRequest.Email, joinRequest.Phone, "") ||
			s.hasJoinRequestConflictLocked(
				joinRequest.RequestedAt,
				joinRequest.ID,
				joinRequest.Username,
				joinRequest.Email,
				joinRequest.Phone,
			) {
			return identity.JoinRequest{}, identity.ErrConflict
		}
	}

	if exists {
		s.deleteJoinRequestIndexes(previous)
	}

	s.joinRequestsByID[joinRequest.ID] = joinRequest
	if joinRequest.Status == identity.JoinRequestStatusPending && joinRequest.ReviewedAt.IsZero() {
		s.indexJoinRequestLocked(joinRequest)
	}

	return joinRequest, nil
}

// JoinRequestByID resolves a join request by its primary key.
func (s *memoryStore) JoinRequestByID(_ context.Context, joinRequestID string) (identity.JoinRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	joinRequest, ok := s.joinRequestsByID[joinRequestID]
	if !ok {
		return identity.JoinRequest{}, identity.ErrNotFound
	}

	return joinRequest, nil
}

// JoinRequestsByStatus returns join requests filtered by lifecycle status.
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
