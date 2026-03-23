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

// failingFirstSaveAccountStore fails the first account write and succeeds afterwards.
type failingFirstSaveAccountStore struct {
	identity.Store

	mu      sync.Mutex
	failed  bool
	failErr error
}

// SaveAccount injects a one-shot failure before delegating to the wrapped store.
func (s *failingFirstSaveAccountStore) SaveAccount(
	ctx context.Context,
	account identity.Account,
) (identity.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.failed {
		s.failed = true
		if s.failErr == nil {
			s.failErr = errors.New("forced create account failure")
		}
		return identity.Account{}, s.failErr
	}

	return s.Store.SaveAccount(ctx, account)
}

func TestCreateAccountIdempotencyKeyDeduplicatesSuccessfulCreation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 22, 0, 0, 0, time.UTC))

	firstAccount, firstBotToken, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:       "create-idem-user",
		DisplayName:    "Create Idem User",
		Email:          "create-idem@example.com",
		AccountKind:    identity.AccountKindUser,
		CreatedBy:      "admin-1",
		IdempotencyKey: "create-account-key",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	secondAccount, secondBotToken, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:       "create-idem-user",
		DisplayName:    "Create Idem User",
		Email:          "create-idem@example.com",
		AccountKind:    identity.AccountKindUser,
		CreatedBy:      "admin-1",
		IdempotencyKey: "create-account-key",
	})
	if err != nil {
		t.Fatalf("repeat create account: %v", err)
	}

	if firstAccount.ID != secondAccount.ID {
		t.Fatalf("expected cached account %s, got %s", firstAccount.ID, secondAccount.ID)
	}
	if firstBotToken != secondBotToken {
		t.Fatalf("expected cached bot token")
	}
}

func TestCreateAccountIdempotencyKeyConflictsOnDifferentRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 22, 15, 0, 0, time.UTC))

	_, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:       "conflict-create-user",
		DisplayName:    "Conflict Create User",
		Email:          "conflict-create-a@example.com",
		AccountKind:    identity.AccountKindUser,
		CreatedBy:      "admin-1",
		IdempotencyKey: "create-conflict-key",
	})
	if err != nil {
		t.Fatalf("first create account: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:       "conflict-create-user-2",
		DisplayName:    "Conflict Create User",
		Email:          "conflict-create-b@example.com",
		AccountKind:    identity.AccountKindUser,
		CreatedBy:      "admin-1",
		IdempotencyKey: "create-conflict-key",
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict on mismatched replay, got %v", err)
	}
}

func TestCreateAccountRetriesAfterSaveFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &failingFirstSaveAccountStore{Store: baseStore}
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 22, 20, 0, 0, time.UTC))

	_, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:       "retry-create-user",
		DisplayName:    "Retry Create User",
		Email:          "retry-create@example.com",
		AccountKind:    identity.AccountKindUser,
		CreatedBy:      "admin-1",
		IdempotencyKey: "retry-create-key",
	})
	if err == nil {
		t.Fatalf("expected create account to fail on first write")
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:       "retry-create-user",
		DisplayName:    "Retry Create User",
		Email:          "retry-create@example.com",
		AccountKind:    identity.AccountKindUser,
		CreatedBy:      "admin-1",
		IdempotencyKey: "retry-create-key",
	})
	if err != nil {
		t.Fatalf("retry create account: %v", err)
	}

	storedAccount, err := baseStore.AccountByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load stored account: %v", err)
	}
	if storedAccount.ID != account.ID {
		t.Fatalf("expected stored account %s, got %s", account.ID, storedAccount.ID)
	}
}

func TestSubmitJoinRequestIdempotencyKeyDeduplicatesSuccessfulRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 22, 30, 0, 0, time.UTC))

	firstJoinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:       "join-idem-user",
		DisplayName:    "Join Idem User",
		Email:          "join-idem@example.com",
		Note:           "please approve",
		IdempotencyKey: "join-request-key",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	secondJoinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:       "join-idem-user",
		DisplayName:    "Join Idem User",
		Email:          "join-idem@example.com",
		Note:           "please approve",
		IdempotencyKey: "join-request-key",
	})
	if err != nil {
		t.Fatalf("repeat submit join request: %v", err)
	}

	if firstJoinRequest.ID != secondJoinRequest.ID {
		t.Fatalf("expected cached join request %s, got %s", firstJoinRequest.ID, secondJoinRequest.ID)
	}
}

func TestApproveJoinRequestIdempotencyKeyDeduplicatesSuccessfulApproval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 22, 45, 0, 0, time.UTC))

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:    "approve-idem-user",
		DisplayName: "Approve Idem User",
		Email:       "approve-idem@example.com",
		Note:        "approved by idempotency test",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	firstJoinRequest, firstAccount, err := svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		Roles:          []identity.Role{identity.RoleAdmin},
		Note:           "approved",
		DecisionReason: "looks good",
		IdempotencyKey: "approve-join-key",
	})
	if err != nil {
		t.Fatalf("approve join request: %v", err)
	}

	secondJoinRequest, secondAccount, err := svc.ApproveJoinRequest(ctx, identity.ApproveJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		Roles:          []identity.Role{identity.RoleAdmin},
		Note:           "approved",
		DecisionReason: "looks good",
		IdempotencyKey: "approve-join-key",
	})
	if err != nil {
		t.Fatalf("repeat approve join request: %v", err)
	}

	if firstJoinRequest.ID != secondJoinRequest.ID {
		t.Fatalf("expected cached join request %s, got %s", firstJoinRequest.ID, secondJoinRequest.ID)
	}
	if firstAccount.ID != secondAccount.ID {
		t.Fatalf("expected cached account %s, got %s", firstAccount.ID, secondAccount.ID)
	}
}

func TestRejectJoinRequestIdempotencyKeyDeduplicatesSuccessfulRejection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 23, 0, 0, 0, time.UTC))

	joinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:    "reject-idem-user",
		DisplayName: "Reject Idem User",
		Email:       "reject-idem@example.com",
		Note:        "reject test",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	firstJoinRequest, err := svc.RejectJoinRequest(ctx, identity.RejectJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		Reason:         "not now",
		IdempotencyKey: "reject-join-key",
	})
	if err != nil {
		t.Fatalf("reject join request: %v", err)
	}

	secondJoinRequest, err := svc.RejectJoinRequest(ctx, identity.RejectJoinRequestParams{
		JoinRequestID:  joinRequest.ID,
		ReviewedBy:     "admin-1",
		Reason:         "not now",
		IdempotencyKey: "reject-join-key",
	})
	if err != nil {
		t.Fatalf("repeat reject join request: %v", err)
	}

	if firstJoinRequest.ID != secondJoinRequest.ID {
		t.Fatalf("expected cached join request %s, got %s", firstJoinRequest.ID, secondJoinRequest.ID)
	}
	if secondJoinRequest.Status != identity.JoinRequestStatusRejected {
		t.Fatalf("expected rejected status, got %s", secondJoinRequest.Status)
	}
}

func TestAuthenticateBotIdempotencyKeyDeduplicatesSuccessfulAuthentication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 23, 15, 0, 0, time.UTC))

	account, botToken, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bot-idem",
		DisplayName: "Bot Idem",
		AccountKind: identity.AccountKindBot,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create bot account: %v", err)
	}
	if botToken == "" {
		t.Fatalf("expected bot token")
	}

	firstResult, err := svc.AuthenticateBot(ctx, identity.AuthenticateBotParams{
		BotToken:       botToken,
		DeviceName:     "bot-device",
		Platform:       identity.DevicePlatformServer,
		PublicKey:      "bot-key",
		ClientVersion:  "1.0.0",
		Locale:         "en",
		IdempotencyKey: "bot-auth-key",
	})
	if err != nil {
		t.Fatalf("authenticate bot: %v", err)
	}

	secondResult, err := svc.AuthenticateBot(ctx, identity.AuthenticateBotParams{
		BotToken:       botToken,
		DeviceName:     "bot-device",
		Platform:       identity.DevicePlatformServer,
		PublicKey:      "bot-key",
		ClientVersion:  "1.0.0",
		Locale:         "en",
		IdempotencyKey: "bot-auth-key",
	})
	if err != nil {
		t.Fatalf("repeat authenticate bot: %v", err)
	}

	if firstResult.Session.ID != secondResult.Session.ID {
		t.Fatalf("expected cached session %s, got %s", firstResult.Session.ID, secondResult.Session.ID)
	}
	if firstResult.Device.ID != secondResult.Device.ID {
		t.Fatalf("expected cached device %s, got %s", firstResult.Device.ID, secondResult.Device.ID)
	}
	if firstResult.Device.AccountID != account.ID {
		t.Fatalf("expected device account %s, got %s", account.ID, firstResult.Device.AccountID)
	}
}
