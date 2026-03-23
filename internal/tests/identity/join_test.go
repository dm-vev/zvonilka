package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestApproveJoinRequestMarksExpiredAndSkipsAccountCreation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 12, 0, 0, 0, time.UTC)

	svc, err := identity.NewService(store, identity.NoopCodeSender{}, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "expired-user",
		Email:    "expired@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	now = now.Add(73 * time.Hour)
	savedJoinRequest, account, err := svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID: joinRequest.ID,
		ReviewedBy:    "admin-1",
	})
	if !errors.Is(err, identity.ErrExpiredJoinRequest) {
		t.Fatalf("expected ErrExpiredJoinRequest, got %v", err)
	}
	if account.ID != "" {
		t.Fatalf("expected no account on expired approval, got %s", account.ID)
	}
	if savedJoinRequest.Status != identity.JoinRequestStatusExpired {
		t.Fatalf("expected expired status, got %s", savedJoinRequest.Status)
	}

	storedJoinRequest, err := store.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load join request: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusExpired {
		t.Fatalf("stored join request should be expired, got %s", storedJoinRequest.Status)
	}

	_, err = store.AccountByUsername(ctx, "expired-user")
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected no account for expired join request, got %v", err)
	}
}

func TestSubmitJoinRequestRejectsExistingAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 12, 15, 0, 0, time.UTC)

	svc, err := identity.NewService(store, identity.NoopCodeSender{}, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "existing-user",
		DisplayName: "Existing User",
		Email:       "existing-user@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	_, err = svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "existing-user",
		Email:    "existing-user@example.com",
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict on duplicate join request, got %v", err)
	}
}

func TestSubmitJoinRequestRejectsDuplicatePendingRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 12, 20, 0, 0, time.UTC)

	svc, err := identity.NewService(store, identity.NoopCodeSender{}, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:    "duplicate-pending-user",
		DisplayName: "Duplicate Pending User",
		Email:       "duplicate-pending@example.com",
		Note:        "first request",
	})
	if err != nil {
		t.Fatalf("submit first join request: %v", err)
	}

	_, err = svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:    "duplicate-pending-user",
		DisplayName: "Duplicate Pending User",
		Email:       "duplicate-pending@example.com",
		Note:        "second request",
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict on duplicate pending join request, got %v", err)
	}
}

func TestSubmitJoinRequestAllowsExpiredPendingRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 12, 25, 0, 0, time.UTC)

	svc, err := identity.NewService(
		store,
		identity.NoopCodeSender{},
		identity.WithNow(func() time.Time {
			return now
		}),
		identity.WithSettings(identity.Settings{
			JoinRequestTTL: time.Hour,
		}),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	firstJoinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:    "expired-resubmit-user",
		DisplayName: "Expired Resubmit User",
		Email:       "expired-resubmit@example.com",
		Note:        "first request",
	})
	if err != nil {
		t.Fatalf("submit first join request: %v", err)
	}

	now = now.Add(2 * time.Hour)

	secondJoinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:    "expired-resubmit-user",
		DisplayName: "Expired Resubmit User",
		Email:       "expired-resubmit@example.com",
		Note:        "second request",
	})
	if err != nil {
		t.Fatalf("submit expired join request again: %v", err)
	}
	if secondJoinRequest.ID == firstJoinRequest.ID {
		t.Fatalf("expected a fresh join request after expiry")
	}

	storedFirstJoinRequest, err := store.JoinRequestByID(ctx, firstJoinRequest.ID)
	if err != nil {
		t.Fatalf("load first join request: %v", err)
	}
	if storedFirstJoinRequest.Status != identity.JoinRequestStatusExpired {
		t.Fatalf("expected first join request to be expired, got %s", storedFirstJoinRequest.Status)
	}
}

func TestRejectJoinRequestMarksExpiredAndSkipsRejection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 12, 30, 0, 0, time.UTC)

	svc, err := identity.NewService(store, identity.NoopCodeSender{}, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "rejected-expired-user",
		Email:    "rejected-expired@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	now = now.Add(73 * time.Hour)
	savedJoinRequest, err := svc.RejectJoinRequest(ctx, identity.RejectJoinRequestParams{
		JoinRequestID: joinRequest.ID,
		ReviewedBy:    "admin-1",
		Reason:        "late review",
	})
	if !errors.Is(err, identity.ErrExpiredJoinRequest) {
		t.Fatalf("expected ErrExpiredJoinRequest, got %v", err)
	}
	if savedJoinRequest.Status != identity.JoinRequestStatusExpired {
		t.Fatalf("expected expired status, got %s", savedJoinRequest.Status)
	}

	storedJoinRequest, err := store.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load join request: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusExpired {
		t.Fatalf("stored join request should be expired, got %s", storedJoinRequest.Status)
	}
}

func TestApproveJoinRequestRollsBackAccountWhenJoinRequestSaveFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &failJoinRequestApprovalStore{Store: baseStore}
	now := time.Date(2026, time.March, 23, 13, 0, 0, 0, time.UTC)

	svc, err := identity.NewService(store, identity.NoopCodeSender{}, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "rollback-user",
		Email:    "rollback@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	_, _, err = svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID: joinRequest.ID,
		ReviewedBy:    "admin-1",
	})
	if !errors.Is(err, errInjectedJoinRequestSave) {
		t.Fatalf("expected injected save failure, got %v", err)
	}

	storedJoinRequest, err := baseStore.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load join request: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusPending {
		t.Fatalf("expected pending join request after failed approval, got %s", storedJoinRequest.Status)
	}

	_, err = baseStore.AccountByUsername(ctx, "rollback-user")
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected account rollback after failed approval, got %v", err)
	}
}

func TestListJoinRequestsByStatusFiltersExpiredPendingRequestsWithoutPersisting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 13, 30, 0, 0, time.UTC)

	svc, err := identity.NewService(store, identity.NoopCodeSender{}, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "list-expired-user",
		Email:    "list-expired@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	now = now.Add(73 * time.Hour)
	pendingJoinRequests, err := svc.ListJoinRequestsByStatus(ctx, identity.JoinRequestStatusPending)
	if err != nil {
		t.Fatalf("list pending join requests: %v", err)
	}
	if len(pendingJoinRequests) != 0 {
		t.Fatalf("expected no pending join requests after filtering, got %d", len(pendingJoinRequests))
	}

	storedJoinRequest, err := store.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load join request: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusPending {
		t.Fatalf("stored join request should remain pending, got %s", storedJoinRequest.Status)
	}
	if !storedJoinRequest.ReviewedAt.IsZero() {
		t.Fatalf("stored join request should not be reviewed during read-only filtering")
	}
}

var errInjectedJoinRequestSave = errors.New("join request save failed")

// failJoinRequestApprovalStore fails approved join-request writes to exercise rollback cleanup.
type failJoinRequestApprovalStore struct {
	identity.Store
}

// WithinTx preserves the injected failure semantics inside transactional callbacks.
func (s *failJoinRequestApprovalStore) WithinTx(ctx context.Context, fn func(identity.Store) error) error {
	return s.Store.WithinTx(ctx, func(tx identity.Store) error {
		return fn(&failJoinRequestApprovalTxStore{Store: tx})
	})
}

// SaveJoinRequest injects a failure for approved join requests.
func (s *failJoinRequestApprovalStore) SaveJoinRequest(
	ctx context.Context,
	joinRequest identity.JoinRequest,
) (identity.JoinRequest, error) {
	if joinRequest.Status == identity.JoinRequestStatusApproved {
		return identity.JoinRequest{}, errInjectedJoinRequestSave
	}

	return s.Store.SaveJoinRequest(ctx, joinRequest)
}

type failJoinRequestApprovalTxStore struct {
	identity.Store
}

func (s *failJoinRequestApprovalTxStore) SaveJoinRequest(
	ctx context.Context,
	joinRequest identity.JoinRequest,
) (identity.JoinRequest, error) {
	if joinRequest.Status == identity.JoinRequestStatusApproved {
		return identity.JoinRequest{}, errInjectedJoinRequestSave
	}

	return s.Store.SaveJoinRequest(ctx, joinRequest)
}
