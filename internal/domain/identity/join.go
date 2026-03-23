package identity

import (
	"context"
	"fmt"
	"time"
)

// ListJoinRequestsByStatus returns join requests for the requested status.
//
// Pending requests are filtered before the result is returned so expired requests do not
// continue to surface as pending business state. The read path stays side-effect free.
func (s *Service) ListJoinRequestsByStatus(
	ctx context.Context,
	status JoinRequestStatus,
) ([]JoinRequest, error) {
	if err := s.validateContext(ctx, "list join requests"); err != nil {
		return nil, err
	}

	joinRequests, err := s.store.JoinRequestsByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("list join requests with status %s: %w", status, err)
	}

	if status != JoinRequestStatusPending {
		return joinRequests, nil
	}

	return s.filterActivePendingJoinRequests(joinRequests)
}

// loadPendingJoinRequest resolves a join request and normalizes it to an actionable state.
//
// Non-pending requests fail fast. Expired requests are persisted as expired so the caller
// can surface the state transition even when the original review happens late.
func (s *Service) loadPendingJoinRequest(
	ctx context.Context,
	store Store,
	joinRequestID string,
	reviewedBy string,
) (JoinRequest, error) {
	joinRequest, err := store.JoinRequestByID(ctx, joinRequestID)
	if err != nil {
		return JoinRequest{}, fmt.Errorf("load join request %s: %w", joinRequestID, err)
	}
	if joinRequest.Status != JoinRequestStatusPending {
		return JoinRequest{}, ErrConflict
	}

	now := s.currentTime()
	if joinRequestExpiredAt(joinRequest, now) {
		expiredJoinRequest, saveErr := s.expireJoinRequest(ctx, store, joinRequest, now, reviewedBy)
		if saveErr != nil {
			return JoinRequest{}, fmt.Errorf("mark join request %s as expired: %w", joinRequest.ID, saveErr)
		}

		return expiredJoinRequest, ErrExpiredJoinRequest
	}

	return joinRequest, nil
}

// filterActivePendingJoinRequests removes expired rows from a pending-only listing.
//
// This keeps the business state clean without turning a list call into a write-heavy
// operation.
func (s *Service) filterActivePendingJoinRequests(joinRequests []JoinRequest) ([]JoinRequest, error) {
	if len(joinRequests) == 0 {
		return joinRequests, nil
	}

	now := s.currentTime()
	activeJoinRequests := make([]JoinRequest, 0, len(joinRequests))
	for _, joinRequest := range joinRequests {
		if !joinRequestExpiredAt(joinRequest, now) {
			activeJoinRequests = append(activeJoinRequests, joinRequest)
		}
	}

	return activeJoinRequests, nil
}

// expireJoinRequest persists the expired status with an audit-friendly reason string.
//
// The helper is called from both approve and reject paths when a moderator acts too late.
func (s *Service) expireJoinRequest(
	ctx context.Context,
	store Store,
	joinRequest JoinRequest,
	now time.Time,
	reviewedBy string,
) (JoinRequest, error) {
	joinRequest.Status = JoinRequestStatusExpired
	joinRequest.ReviewedAt = now
	joinRequest.ReviewedBy = trimmed(reviewedBy)
	joinRequest.DecisionReason = "join request expired"

	savedJoinRequest, err := store.SaveJoinRequest(ctx, joinRequest)
	if err != nil {
		return JoinRequest{}, fmt.Errorf("save join request %s: %w", joinRequest.ID, err)
	}

	return savedJoinRequest, nil
}

// joinRequestExpiredAt reports whether a join request has reached or passed its TTL.
func joinRequestExpiredAt(joinRequest JoinRequest, now time.Time) bool {
	if joinRequest.ExpiresAt.IsZero() {
		return false
	}

	return !now.Before(joinRequest.ExpiresAt)
}
