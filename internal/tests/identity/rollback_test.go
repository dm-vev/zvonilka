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

// onceFailingApprovalStore fails the first approved join-request write.
type onceFailingApprovalStore struct {
	identity.Store

	mu     sync.Mutex
	failed bool
}

// WithinTx preserves the injected failure semantics inside transactional callbacks.
func (s *onceFailingApprovalStore) WithinTx(ctx context.Context, fn func(identity.Store) error) error {
	return s.Store.WithinTx(ctx, func(tx identity.Store) error {
		return fn(&onceFailingApprovalTxStore{
			Store:  tx,
			parent: s,
		})
	})
}

// SaveJoinRequest injects a single approved-write failure.
func (s *onceFailingApprovalStore) SaveJoinRequest(
	ctx context.Context,
	joinRequest identity.JoinRequest,
) (identity.JoinRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if joinRequest.Status == identity.JoinRequestStatusApproved && !s.failed {
		s.failed = true
		return identity.JoinRequest{}, errApprovedJoinRequestSave
	}

	return s.Store.SaveJoinRequest(ctx, joinRequest)
}

type onceFailingApprovalTxStore struct {
	identity.Store

	parent *onceFailingApprovalStore
}

func (s *onceFailingApprovalTxStore) SaveJoinRequest(
	ctx context.Context,
	joinRequest identity.JoinRequest,
) (identity.JoinRequest, error) {
	s.parent.mu.Lock()
	defer s.parent.mu.Unlock()

	if joinRequest.Status == identity.JoinRequestStatusApproved && !s.parent.failed {
		s.parent.failed = true
		return identity.JoinRequest{}, errApprovedJoinRequestSave
	}

	return s.Store.SaveJoinRequest(ctx, joinRequest)
}

func TestApproveJoinRequestRetryRecoversAfterTransactionRollback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &onceFailingApprovalStore{Store: baseStore}
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 24, 0, 15, 0, 0, time.UTC))

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "approval-tx-retry-user",
		Email:    "approval-tx-retry@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	_, _, err = svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-tx-retry-key",
	})
	if !errors.Is(err, errApprovedJoinRequestSave) {
		t.Fatalf("expected approval save failure, got %v", err)
	}

	if _, err := baseStore.AccountByUsername(ctx, joinRequest.Username); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected no persisted account after rollback, got %v", err)
	}

	storedJoinRequest, err := baseStore.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load join request after rollback: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusPending {
		t.Fatalf("expected join request to remain pending, got %s", storedJoinRequest.Status)
	}

	approvedJoinRequest, account, err := svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-tx-retry-key",
	})
	if err != nil {
		t.Fatalf("retry approve join request: %v", err)
	}
	if approvedJoinRequest.Status != identity.JoinRequestStatusApproved {
		t.Fatalf("expected approved join request, got %s", approvedJoinRequest.Status)
	}
	if account.ID == "" {
		t.Fatalf("expected approved account on retry")
	}

	storedAccount, err := baseStore.AccountByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load approved account: %v", err)
	}
	if storedAccount.ID != account.ID {
		t.Fatalf("expected stored account %s, got %s", account.ID, storedAccount.ID)
	}
}

func TestApproveJoinRequestRetryAfterCacheExpiryCreatesFreshAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &onceFailingApprovalStore{Store: baseStore}
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
	if !errors.Is(err, errApprovedJoinRequestSave) {
		t.Fatalf("expected approval save failure, got %v", err)
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
		t.Fatalf("expected created account after cache expiry")
	}

	storedAccount, err := baseStore.AccountByUsername(ctx, joinRequest.Username)
	if err != nil {
		t.Fatalf("load account after cache expiry: %v", err)
	}
	if storedAccount.ID != recoveredAccount.ID {
		t.Fatalf("expected stored account %s, got %s", recoveredAccount.ID, storedAccount.ID)
	}
}

func TestApproveJoinRequestRejectsExistingAccountWithMismatchedRoles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 24, 2, 0, 0, 0, time.UTC))

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:    "approval-role-mismatch-user",
		DisplayName: "Approval Role Mismatch User",
		Email:       "approval-role-mismatch@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	if _, err := store.SaveAccount(ctx, identity.Account{
		ID:          "acc-role-mismatch",
		Kind:        identity.AccountKindUser,
		Username:    joinRequest.Username,
		DisplayName: joinRequest.DisplayName,
		Email:       joinRequest.Email,
		Phone:       joinRequest.Phone,
		Roles:       []identity.Role{identity.RoleModerator},
		Status:      identity.AccountStatusActive,
		CreatedBy:   "admin-1",
	}); err != nil {
		t.Fatalf("save conflicting account: %v", err)
	}

	_, _, err = svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID: joinRequest.ID,
		ReviewedBy:    "admin-1",
		Roles:         []identity.Role{identity.RoleAdmin},
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict for mismatched recovered account, got %v", err)
	}

	storedJoinRequest, err := store.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load join request: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusPending {
		t.Fatalf("expected join request to remain pending, got %s", storedJoinRequest.Status)
	}
}
