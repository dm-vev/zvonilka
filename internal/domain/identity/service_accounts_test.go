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

func TestListJoinRequestsByStatusExpiresPendingRequestsBeforeReturning(t *testing.T) {
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
		t.Fatalf("expected no pending join requests after cleanup, got %d", len(pendingJoinRequests))
	}

	storedJoinRequest, err := store.JoinRequestByID(ctx, joinRequest.ID)
	if err != nil {
		t.Fatalf("load join request: %v", err)
	}
	if storedJoinRequest.Status != identity.JoinRequestStatusExpired {
		t.Fatalf("stored join request should be expired, got %s", storedJoinRequest.Status)
	}
}

var errInjectedJoinRequestSave = errors.New("join request save failed")

type failJoinRequestApprovalStore struct {
	identity.Store
}

func (s *failJoinRequestApprovalStore) SaveJoinRequest(
	ctx context.Context,
	joinRequest identity.JoinRequest,
) (identity.JoinRequest, error) {
	if joinRequest.Status == identity.JoinRequestStatusApproved {
		return identity.JoinRequest{}, errInjectedJoinRequestSave
	}

	return s.Store.SaveJoinRequest(ctx, joinRequest)
}
