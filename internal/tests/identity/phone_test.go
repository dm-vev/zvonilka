package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestCreateAccountRejectsDuplicatePhoneAcrossFormatting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 20, 0, 0, 0, time.UTC)
	svc, err := identity.NewService(
		store,
		identity.NoopCodeSender{},
		identity.WithNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "phone-owner-1",
		DisplayName: "Phone Owner 1",
		Phone:       "+1 (555) 0100",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create first account: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "phone-owner-2",
		DisplayName: "Phone Owner 2",
		Phone:       "1 555-0100",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected ErrConflict for duplicate phone, got %v", err)
	}
}

func TestBeginLoginMatchesPhoneAcrossFormatting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 20, 5, 0, 0, time.UTC)
	svc, err := identity.NewService(
		store,
		identity.NoopCodeSender{},
		identity.WithNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "phone-login-user",
		DisplayName: "Phone Login User",
		Phone:       "+1 (555) 0100",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Phone:    "1-555-0100",
		Delivery: identity.LoginDeliveryChannelSMS,
	})
	if err != nil {
		t.Fatalf("begin login by alternate phone format: %v", err)
	}

	if challenge.AccountID != account.ID {
		t.Fatalf("expected challenge account %s, got %s", account.ID, challenge.AccountID)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 login target, got %d", len(targets))
	}
	if targets[0].Channel != identity.LoginDeliveryChannelSMS {
		t.Fatalf("expected SMS login target, got %s", targets[0].Channel)
	}
}

func TestBeginLoginMasksUnicodePhoneWithoutBreakingUTF8(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 23, 20, 10, 0, 0, time.UTC)
	svc, err := identity.NewService(
		store,
		identity.NoopCodeSender{},
		identity.WithNow(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "unicode-phone-user",
		DisplayName: "Unicode Phone User",
		Phone:       "١٢٣٤٥٦٧٨٩",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	_, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Phone:    "١٢٣٤٥٦٧٨٩",
		Delivery: identity.LoginDeliveryChannelSMS,
	})
	if err != nil {
		t.Fatalf("begin login by unicode phone: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 login target, got %d", len(targets))
	}
	if !utf8.ValidString(targets[0].DestinationMask) {
		t.Fatalf("expected valid utf-8 destination mask, got %q", targets[0].DestinationMask)
	}
	if targets[0].DestinationMask == "" {
		t.Fatalf("expected non-empty destination mask")
	}
}
