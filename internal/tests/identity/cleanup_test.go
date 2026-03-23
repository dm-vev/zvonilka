package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestIdempotencyCacheCleanupIsBucketScoped(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 23, 45, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time {
		return now
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	firstJoinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:       "cleanup-submit",
		DisplayName:    "Cleanup Submit",
		Email:          "cleanup-submit@example.com",
		IdempotencyKey: "submit-key",
	})
	if err != nil {
		t.Fatalf("submit join request: %v", err)
	}

	now = now.Add(12*time.Hour + time.Minute)

	account, botToken, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:       "cleanup-create",
		DisplayName:    "Cleanup Create",
		Email:          "cleanup-create@example.com",
		AccountKind:    identity.AccountKindUser,
		CreatedBy:      "admin-1",
		IdempotencyKey: "create-key",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       account.Username,
		Delivery:       identity.LoginDeliveryChannelEmail,
		DeviceName:     "Cleanup Device",
		Platform:       identity.DevicePlatformIOS,
		ClientVersion:  "1.0.0",
		Locale:         "en",
		IdempotencyKey: "begin-key",
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one login target, got %d", len(targets))
	}

	cachedSendCount := sender.totalSends()

	now = now.Add(12*time.Hour + time.Minute)

	repeatedJoinRequest, err := svc.SubmitJoinRequest(ctx, identity.SubmitJoinRequestParams{
		Username:       "cleanup-submit-2",
		DisplayName:    "Cleanup Submit Two",
		Email:          "cleanup-submit-2@example.com",
		IdempotencyKey: "submit-key",
	})
	if err != nil {
		t.Fatalf("repeat submit join request: %v", err)
	}
	if repeatedJoinRequest.ID == firstJoinRequest.ID {
		t.Fatalf("expected expired submit cache to produce a new join request")
	}

	repeatedAccount, repeatedBotToken, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:       "cleanup-create",
		DisplayName:    "Cleanup Create",
		Email:          "cleanup-create@example.com",
		AccountKind:    identity.AccountKindUser,
		CreatedBy:      "admin-1",
		IdempotencyKey: "create-key",
	})
	if err != nil {
		t.Fatalf("repeat create account: %v", err)
	}
	if repeatedAccount.ID != account.ID {
		t.Fatalf("expected cached account %s, got %s", account.ID, repeatedAccount.ID)
	}
	if repeatedBotToken != botToken {
		t.Fatalf("expected cached bot token %q, got %q", botToken, repeatedBotToken)
	}

	repeatedChallenge, repeatedTargets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       account.Username,
		Delivery:       identity.LoginDeliveryChannelEmail,
		DeviceName:     "Cleanup Device",
		Platform:       identity.DevicePlatformIOS,
		ClientVersion:  "1.0.0",
		Locale:         "en",
		IdempotencyKey: "begin-key",
	})
	if err != nil {
		t.Fatalf("repeat begin login: %v", err)
	}
	if repeatedChallenge.ID != challenge.ID {
		t.Fatalf("expected cached challenge %s, got %s", challenge.ID, repeatedChallenge.ID)
	}
	if len(repeatedTargets) != len(targets) {
		t.Fatalf("expected %d cached targets, got %d", len(targets), len(repeatedTargets))
	}
	if sender.totalSends() != cachedSendCount {
		t.Fatalf("expected no extra login code delivery, got %d sends", sender.totalSends())
	}
}
