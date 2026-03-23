package identity_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

var errApprovalRollbackDeleteFailure = errors.New("forced approval rollback delete failure")

type onceFailingApprovalRollbackStore struct {
	identity.Store

	mu           sync.Mutex
	saveFailed   bool
	deleteFailed bool
}

func (s *onceFailingApprovalRollbackStore) SaveJoinRequest(
	ctx context.Context,
	joinRequest identity.JoinRequest,
) (identity.JoinRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if joinRequest.Status == identity.JoinRequestStatusApproved && !s.saveFailed {
		s.saveFailed = true
		return identity.JoinRequest{}, errApprovedJoinRequestSave
	}

	return s.Store.SaveJoinRequest(ctx, joinRequest)
}

func (s *onceFailingApprovalRollbackStore) DeleteAccount(ctx context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.deleteFailed {
		s.deleteFailed = true
		return errApprovalRollbackDeleteFailure
	}

	return s.Store.DeleteAccount(ctx, accountID)
}

type alwaysFailApprovedJoinRequestDeleteOnceStore struct {
	identity.Store

	mu           sync.Mutex
	deleteFailed bool
}

func (s *alwaysFailApprovedJoinRequestDeleteOnceStore) SaveJoinRequest(
	ctx context.Context,
	joinRequest identity.JoinRequest,
) (identity.JoinRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if joinRequest.Status == identity.JoinRequestStatusApproved {
		return identity.JoinRequest{}, errApprovedJoinRequestSave
	}

	return s.Store.SaveJoinRequest(ctx, joinRequest)
}

func (s *alwaysFailApprovedJoinRequestDeleteOnceStore) DeleteAccount(ctx context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.deleteFailed {
		s.deleteFailed = true
		return errApprovalRollbackDeleteFailure
	}

	return s.Store.DeleteAccount(ctx, accountID)
}

func TestApproveJoinRequestRetryRecoversAfterRollbackDeleteFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &onceFailingApprovalRollbackStore{Store: baseStore}
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 24, 0, 15, 0, 0, time.UTC))

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "approval-delete-retry-user",
		Email:    "approval-delete-retry@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	_, _, err = svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-delete-retry-key",
	})
	if !errors.Is(err, errApprovalRollbackDeleteFailure) {
		t.Fatalf("expected rollback delete failure, got %v", err)
	}

	orphanAccount, err := baseStore.AccountByUsername(ctx, joinRequest.Username)
	if err != nil {
		t.Fatalf("load orphan account: %v", err)
	}
	if orphanAccount.ID == "" {
		t.Fatalf("expected orphan account to remain after rollback failure")
	}

	recoveredJoinRequest, recoveredAccount, err := svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-delete-retry-key",
	})
	if err != nil {
		t.Fatalf("retry approve join request: %v", err)
	}
	if recoveredJoinRequest.Status != identity.JoinRequestStatusApproved {
		t.Fatalf("expected approved join request, got %s", recoveredJoinRequest.Status)
	}
	if recoveredAccount.ID != orphanAccount.ID {
		t.Fatalf("expected retry to reuse orphan account %s, got %s", orphanAccount.ID, recoveredAccount.ID)
	}

	storedJoinRequest, err := baseStore.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load approved join request: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusApproved {
		t.Fatalf("expected stored join request approved, got %s", storedJoinRequest.Status)
	}
}

func TestApproveJoinRequestRetryRecoversAfterRollbackDeleteFailureAndCacheExpiry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &onceFailingApprovalRollbackStore{Store: baseStore}
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 24, 1, 0, 0, 0, time.UTC)
	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "approval-cache-expiry-user",
		Email:    "approval-cache-expiry@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	_, _, err = svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-cache-expiry-key",
	})
	if !errors.Is(err, errApprovalRollbackDeleteFailure) {
		t.Fatalf("expected rollback delete failure, got %v", err)
	}

	now = now.Add(24*time.Hour + time.Minute)

	recoveredJoinRequest, recoveredAccount, err := svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-cache-expiry-key",
	})
	if err != nil {
		t.Fatalf("retry approve join request after cache expiry: %v", err)
	}
	if recoveredJoinRequest.Status != identity.JoinRequestStatusApproved {
		t.Fatalf("expected approved join request, got %s", recoveredJoinRequest.Status)
	}
	if recoveredAccount.ID == "" {
		t.Fatalf("expected recovered account after cache expiry")
	}

	storedJoinRequest, err := baseStore.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load approved join request: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusApproved {
		t.Fatalf("expected stored join request approved, got %s", storedJoinRequest.Status)
	}

	storedAccount, err := baseStore.AccountByUsername(ctx, joinRequest.Username)
	if err != nil {
		t.Fatalf("load recovered account: %v", err)
	}
	if storedAccount.ID != recoveredAccount.ID {
		t.Fatalf("expected recovered account %s, got %s", recoveredAccount.ID, storedAccount.ID)
	}
}

func TestApproveJoinRequestRetryKeepsRecoveredAccountOnSaveFailureAfterCacheExpiry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &alwaysFailApprovedJoinRequestDeleteOnceStore{Store: baseStore}
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 24, 1, 30, 0, 0, time.UTC)
	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "approval-recovered-account-user",
		Email:    "approval-recovered-account@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	_, _, err = svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-recovered-account-key",
	})
	if !errors.Is(err, errApprovalRollbackDeleteFailure) {
		t.Fatalf("expected rollback delete failure, got %v", err)
	}

	now = now.Add(24*time.Hour + time.Minute)

	_, _, err = svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-recovered-account-key",
	})
	if !errors.Is(err, errApprovedJoinRequestSave) {
		t.Fatalf("expected approved join request save failure, got %v", err)
	}

	storedAccount, err := baseStore.AccountByUsername(ctx, joinRequest.Username)
	if err != nil {
		t.Fatalf("load recovered account: %v", err)
	}
	if storedAccount.ID == "" {
		t.Fatalf("expected recovered account to remain after failed retry")
	}

	storedJoinRequest, err := baseStore.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load join request after failed retry: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusPending {
		t.Fatalf("expected join request to remain pending after failed retry, got %s", storedJoinRequest.Status)
	}
}
