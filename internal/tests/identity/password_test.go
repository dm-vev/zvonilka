package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestCreatePasswordOnlyAccountSupportsPasswordLogin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 24, 2, 0, 0, 0, time.UTC))

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "password-only-user",
		DisplayName: "Password Only User",
		Password:    "s3cret-pass",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create password-only account: %v", err)
	}
	if account.Email != "" || account.Phone != "" {
		t.Fatalf("expected password-only account without contacts, got email=%q phone=%q", account.Email, account.Phone)
	}

	options, err := svc.GetLoginOptions(ctx, identity.GetLoginOptionsParams{
		Username: account.Username,
	})
	if err != nil {
		t.Fatalf("get login options: %v", err)
	}
	if len(options.Options) != 1 {
		t.Fatalf("expected one login option, got %d", len(options.Options))
	}
	if options.Options[0].Factor != identity.LoginFactorPassword {
		t.Fatalf("expected password login option, got %s", options.Options[0].Factor)
	}

	result, err := svc.AuthenticatePassword(ctx, identity.AuthenticatePasswordParams{
		Username:   account.Username,
		Password:   "s3cret-pass",
		DeviceName: "web",
		Platform:   identity.DevicePlatformWeb,
		PublicKey:  "password-only-public-key",
	})
	if err != nil {
		t.Fatalf("authenticate password: %v", err)
	}
	if result.Session.AccountID != account.ID {
		t.Fatalf("expected session for account %s, got %s", account.ID, result.Session.AccountID)
	}
	if result.Device.ID == "" || result.Session.ID == "" {
		t.Fatal("expected device and session to be issued")
	}
}

func TestAuthenticatePasswordRejectsMissingCredential(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 24, 2, 5, 0, 0, time.UTC))

	account := createUserAccount(t, svc, ctx, "no-password-user")
	_, err := svc.AuthenticatePassword(ctx, identity.AuthenticatePasswordParams{
		Username:   account.Username,
		Password:   "missing-pass",
		DeviceName: "web",
		Platform:   identity.DevicePlatformWeb,
		PublicKey:  "no-password-public-key",
	})
	if !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for missing password credential, got %v", err)
	}
}

func TestAuthenticatePasswordIdempotencyReturnsCachedLoginResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 24, 2, 10, 0, 0, time.UTC))

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "password-idempotency-user",
		DisplayName: "Password Idempotency User",
		Email:       "password-idempotency@example.com",
		Password:    "cached-pass",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	firstResult, err := svc.AuthenticatePassword(ctx, identity.AuthenticatePasswordParams{
		Username:       account.Username,
		Password:       "cached-pass",
		DeviceName:     "desktop",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "password-idempotency-public-key",
		IdempotencyKey: "password-login-key",
	})
	if err != nil {
		t.Fatalf("first password login: %v", err)
	}

	secondResult, err := svc.AuthenticatePassword(ctx, identity.AuthenticatePasswordParams{
		Username:       account.Username,
		Password:       "cached-pass",
		DeviceName:     "desktop",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "password-idempotency-public-key",
		IdempotencyKey: "password-login-key",
	})
	if err != nil {
		t.Fatalf("second password login: %v", err)
	}

	if firstResult.Session.ID != secondResult.Session.ID {
		t.Fatalf("expected cached session %s, got %s", firstResult.Session.ID, secondResult.Session.ID)
	}
	if firstResult.Device.ID != secondResult.Device.ID {
		t.Fatalf("expected cached device %s, got %s", firstResult.Device.ID, secondResult.Device.ID)
	}
	if firstResult.Tokens != secondResult.Tokens {
		t.Fatal("expected cached tokens")
	}

	devices, err := store.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices after cached password login: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected one device after cached password login, got %d", len(devices))
	}
}
