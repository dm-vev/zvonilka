package identity_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

// overwriteConcurrentAccountMetadata writes a fresh metadata snapshot for the
// account while the boundary test is paused inside the revoke/login/device path.
func overwriteConcurrentAccountMetadata(
	t *testing.T,
	ctx context.Context,
	store identity.Store,
	accountID string,
	clock *steppedClock,
) error {
	t.Helper()

	account, err := store.AccountByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("load account for concurrent update: %w", err)
	}

	account.DisplayName = "Concurrent Display Name"
	account.Bio = "concurrent bio"
	account.CustomBadgeEmoji = "🛰️"
	account.UpdatedAt = clock.Now()

	if _, err := store.SaveAccount(ctx, account); err != nil {
		return fmt.Errorf("save concurrent account update: %w", err)
	}

	return nil
}

// TestRevokeAllSessionsPreservesConcurrentAccountMetadataUpdate proves that the
// account boundary does not clobber a concurrent profile edit.
func TestRevokeAllSessionsPreservesConcurrentAccountMetadataUpdate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := newAccountGateStore(baseStore)
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 24, 4, 0, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "boundary-user",
		DisplayName: "Boundary User",
		Email:       "boundary@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	// Create an active session so the revoke path has work to do after the
	// account boundary is touched.
	session := newLoggedInAccount(t, svc, sender, account.Username, "boundary-phone-1", "boundary-login-key")
	if session.ID == "" {
		t.Fatalf("expected active session")
	}

	store.enableBlocking()

	revokeDone := make(chan error, 1)
	go func() {
		_, err := svc.RevokeAllSessions(ctx, account.ID, identity.RevokeAllSessionsParams{
			Reason:         "revoke all",
			IdempotencyKey: "boundary-revoke-key",
		})
		revokeDone <- err
	}()

	<-store.entered

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- overwriteConcurrentAccountMetadata(t, ctx, store, account.ID, clock)
	}()

	<-store.saveEntered

	close(store.release)

	if err := <-revokeDone; err != nil {
		t.Fatalf("revoke all sessions: %v", err)
	}
	if err := <-updateDone; err != nil {
		t.Fatalf("concurrent account update: %v", err)
	}

	persistedAccount, err := baseStore.AccountByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("reload account after revoke: %v", err)
	}
	if persistedAccount.DisplayName != "Concurrent Display Name" {
		t.Fatalf("expected concurrent display name to survive, got %q", persistedAccount.DisplayName)
	}
	if persistedAccount.Bio != "concurrent bio" {
		t.Fatalf("expected concurrent bio to survive, got %q", persistedAccount.Bio)
	}
	if persistedAccount.CustomBadgeEmoji != "🛰️" {
		t.Fatalf("expected concurrent badge emoji to survive, got %q", persistedAccount.CustomBadgeEmoji)
	}
}

// TestVerifyLoginCodePreservesConcurrentAccountMetadataUpdate proves that login
// boundary touches do not overwrite a concurrent account profile edit.
func TestVerifyLoginCodePreservesConcurrentAccountMetadataUpdate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := newAccountGateStore(baseStore)
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 24, 4, 30, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "boundary-login-user",
		DisplayName: "Boundary Login User",
		Email:       "boundary-login@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username: account.Username,
		Delivery: identity.LoginDeliveryChannelEmail,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one login target, got %d", len(targets))
	}

	code := sender.codeFor(targets[0].DestinationMask)
	if code == "" {
		t.Fatalf("expected recorded login code")
	}
	_ = challenge

	store.enableBlocking()

	verifyDone := make(chan error, 1)
	go func() {
		_, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
			ChallengeID: challenge.ID,
			Code:        code,
			DeviceName:  "boundary laptop",
			Platform:    identity.DevicePlatformDesktop,
			PublicKey:   "boundary-login-key",
		})
		verifyDone <- err
	}()

	<-store.entered

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- overwriteConcurrentAccountMetadata(t, ctx, store, account.ID, clock)
	}()

	<-store.saveEntered
	close(store.release)

	if err := <-verifyDone; err != nil {
		t.Fatalf("verify login code: %v", err)
	}
	if err := <-updateDone; err != nil {
		t.Fatalf("concurrent account update: %v", err)
	}

	persistedAccount, err := baseStore.AccountByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("reload account after login: %v", err)
	}
	if persistedAccount.DisplayName != "Concurrent Display Name" {
		t.Fatalf("expected concurrent display name to survive, got %q", persistedAccount.DisplayName)
	}
	if persistedAccount.Bio != "concurrent bio" {
		t.Fatalf("expected concurrent bio to survive, got %q", persistedAccount.Bio)
	}
	if persistedAccount.CustomBadgeEmoji != "🛰️" {
		t.Fatalf("expected concurrent badge emoji to survive, got %q", persistedAccount.CustomBadgeEmoji)
	}
}

// TestRegisterDevicePreservesConcurrentAccountMetadataUpdate proves that device
// registration boundary touches do not overwrite a concurrent account profile edit.
func TestRegisterDevicePreservesConcurrentAccountMetadataUpdate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := newAccountGateStore(baseStore)
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 24, 5, 0, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "boundary-register-user",
		DisplayName: "Boundary Register User",
		Email:       "boundary-register@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	session := newLoggedInAccount(t, svc, sender, account.Username, "boundary-phone-2", "boundary-register-key")
	if session.ID == "" {
		t.Fatalf("expected active session")
	}

	store.enableBlocking()

	registerDone := make(chan error, 1)
	go func() {
		_, _, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
			SessionID:      session.ID,
			DeviceName:     "boundary tablet",
			Platform:       identity.DevicePlatformDesktop,
			PublicKey:      "boundary-register-device-key",
			IdempotencyKey: "boundary-register-idem",
		})
		registerDone <- err
	}()

	<-store.entered

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- overwriteConcurrentAccountMetadata(t, ctx, store, account.ID, clock)
	}()

	<-store.saveEntered
	close(store.release)

	if err := <-registerDone; err != nil {
		t.Fatalf("register device: %v", err)
	}
	if err := <-updateDone; err != nil {
		t.Fatalf("concurrent account update: %v", err)
	}

	persistedAccount, err := baseStore.AccountByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("reload account after register: %v", err)
	}
	if persistedAccount.DisplayName != "Concurrent Display Name" {
		t.Fatalf("expected concurrent display name to survive, got %q", persistedAccount.DisplayName)
	}
	if persistedAccount.Bio != "concurrent bio" {
		t.Fatalf("expected concurrent bio to survive, got %q", persistedAccount.Bio)
	}
	if persistedAccount.CustomBadgeEmoji != "🛰️" {
		t.Fatalf("expected concurrent badge emoji to survive, got %q", persistedAccount.CustomBadgeEmoji)
	}
}
