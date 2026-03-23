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

var errApprovedJoinRequestSave = errors.New("forced approved join request save failure")

// onceFailingApprovedJoinRequestStore fails the first approved join-request write.
type onceFailingApprovedJoinRequestStore struct {
	identity.Store

	mu     sync.Mutex
	failed bool
}

// SaveJoinRequest injects a one-shot failure for approved join requests.
func (s *onceFailingApprovedJoinRequestStore) SaveJoinRequest(
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

func TestApproveJoinRequestRetryRecreatesRolledBackAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &onceFailingApprovedJoinRequestStore{Store: baseStore}
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 23, 30, 0, 0, time.UTC))

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username: "approval-retry-user",
		Email:    "approval-retry@example.com",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	_, _, err = svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-retry-key",
	})
	if !errors.Is(err, errApprovedJoinRequestSave) {
		t.Fatalf("expected approval save failure, got %v", err)
	}

	approvedJoinRequest, account, err := svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		IdempotencyKey: "approve-retry-key",
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

	storedByUsername, err := baseStore.AccountByUsername(ctx, joinRequest.Username)
	if err != nil {
		t.Fatalf("load approved account by username: %v", err)
	}
	if storedByUsername.ID != account.ID {
		t.Fatalf("expected username lookup to return account %s, got %s", account.ID, storedByUsername.ID)
	}
}
