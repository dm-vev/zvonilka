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

// failingCodeSender fails the first login-code delivery and succeeds afterwards.
type failingCodeSender struct {
	mu       sync.Mutex
	attempts int
	failErr  error
}

// SendLoginCode injects a one-shot delivery failure.
func (s *failingCodeSender) SendLoginCode(_ context.Context, _ identity.LoginTarget, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.attempts++
	if s.attempts == 1 {
		if s.failErr == nil {
			s.failErr = errors.New("forced login code send failure")
		}
		return s.failErr
	}

	return nil
}

// totalAttempts returns the number of observed delivery attempts.
func (s *failingCodeSender) totalAttempts() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.attempts
}

// failingUpdateSessionStore fails the first UpdateSession call to exercise rollback.
type failingUpdateSessionStore struct {
	identity.Store
	failErr    error
	failOnCall int
	calls      int
}

// WithinTx preserves the injected failure semantics inside transactional callbacks.
func (s *failingUpdateSessionStore) WithinTx(ctx context.Context, fn func(identity.Store) error) error {
	return s.Store.WithinTx(ctx, func(tx identity.Store) error {
		return fn(&failingUpdateSessionTxStore{
			Store:  tx,
			parent: s,
		})
	})
}

// UpdateSession injects a one-shot failure before delegating to the wrapped store.
func (s *failingUpdateSessionStore) UpdateSession(ctx context.Context, session identity.Session) (identity.Session, error) {
	if s.failErr == nil {
		s.failErr = errors.New("forced update session failure")
	}
	s.calls++
	failOnCall := s.failOnCall
	if failOnCall == 0 {
		failOnCall = 1
	}
	if s.calls == failOnCall {
		return identity.Session{}, s.failErr
	}

	return s.Store.UpdateSession(ctx, session)
}

type failingUpdateSessionTxStore struct {
	identity.Store

	parent *failingUpdateSessionStore
}

func (s *failingUpdateSessionTxStore) UpdateSession(ctx context.Context, session identity.Session) (identity.Session, error) {
	if s.parent.failErr == nil {
		s.parent.failErr = errors.New("forced update session failure")
	}
	s.parent.calls++
	failOnCall := s.parent.failOnCall
	if failOnCall == 0 {
		failOnCall = 1
	}
	if s.parent.calls == failOnCall {
		return identity.Session{}, s.parent.failErr
	}

	return s.Store.UpdateSession(ctx, session)
}

// createAccount builds a minimal user account for reliability tests.
func createAccount(t *testing.T, svc *identity.Service, ctx context.Context, username, email string) identity.Account {
	t.Helper()

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    username,
		DisplayName: username,
		Email:       email,
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	return account
}

// beginLogin starts a login challenge and asserts the expected target count.
func beginLogin(
	t *testing.T,
	svc *identity.Service,
	ctx context.Context,
	username string,
	idempotencyKey string,
) (identity.LoginChallenge, []identity.LoginTarget) {
	t.Helper()

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       username,
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one login target, got %d", len(targets))
	}

	return challenge, targets
}

// verifyLogin completes a login challenge and returns the issued session bundle.
func verifyLogin(
	t *testing.T,
	svc *identity.Service,
	ctx context.Context,
	challengeID string,
	code string,
	idempotencyKey string,
	deviceName string,
) identity.LoginResult {
	t.Helper()

	result, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challengeID,
		Code:           code,
		DeviceName:     deviceName,
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "device-key",
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	return result
}

func TestBeginLoginDeduplicatesSuccessfulRequestsByIdempotencyKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 0, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	createAccount(t, svc, ctx, "begin-idem-user", "begin-idem@example.com")

	challenge1, targets1, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       "begin-idem-user",
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-idem-key",
	})
	if err != nil {
		t.Fatalf("begin login first call: %v", err)
	}

	challenge2, targets2, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       "begin-idem-user",
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-idem-key",
	})
	if err != nil {
		t.Fatalf("begin login second call: %v", err)
	}

	if challenge1.ID != challenge2.ID {
		t.Fatalf("expected cached challenge ID %s, got %s", challenge1.ID, challenge2.ID)
	}
	if len(targets1) != len(targets2) || len(targets1) != 1 {
		t.Fatalf("expected one target in both responses, got %d and %d", len(targets1), len(targets2))
	}
	if sender.totalSends() != 1 {
		t.Fatalf("expected one code send, got %d", sender.totalSends())
	}
}

func TestBeginLoginRollsBackChallengeWhenSendFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &failingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 5, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	createAccount(t, svc, ctx, "begin-rollback-user", "begin-rollback@example.com")

	_, _, err = svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       "begin-rollback-user",
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-rollback-key",
	})
	if err == nil {
		t.Fatalf("expected first begin login to fail")
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       "begin-rollback-user",
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-rollback-key",
	})
	if err != nil {
		t.Fatalf("expected retry to succeed after rollback, got %v", err)
	}
	if challenge.ID == "" {
		t.Fatalf("expected challenge on retry")
	}
	if len(targets) != 1 {
		t.Fatalf("expected one login target on retry, got %d", len(targets))
	}
	if sender.totalAttempts() != 2 {
		t.Fatalf("expected two send attempts, got %d", sender.totalAttempts())
	}
}

func TestVerifyLoginCodeExpiresAtBoundary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 8, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account := createAccount(t, svc, ctx, "expired-boundary-user", "expired-boundary@example.com")
	challenge, targets := beginLogin(t, svc, ctx, account.Username, "expired-boundary-begin")
	code := sender.codeFor(targets[0].DestinationMask)

	now = now.Add(10 * time.Minute)

	_, err = svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        code,
		DeviceName:  "boundary-device",
		Platform:    identity.DevicePlatformIOS,
		PublicKey:   "boundary-key",
	})
	if !errors.Is(err, identity.ErrExpiredChallenge) {
		t.Fatalf("expected ErrExpiredChallenge at boundary, got %v", err)
	}

	storedChallenge, err := store.LoginChallengeByID(ctx, challenge.ID)
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected expired challenge to be deleted, got %v", err)
	}
	if storedChallenge.ID != "" {
		t.Fatalf("expected no stored challenge after expiry, got %s", storedChallenge.ID)
	}

	devices, err := store.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices after expired verify: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected no devices after expired verify, got %d", len(devices))
	}
}

func TestVerifyLoginCodeDeduplicatesSuccessfulRequestsByIdempotencyKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 10, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account := createAccount(t, svc, ctx, "verify-idem-user", "verify-idem@example.com")
	challenge, targets := beginLogin(t, svc, ctx, account.Username, "")
	code := sender.codeFor(targets[0].DestinationMask)

	result1 := verifyLogin(t, svc, ctx, challenge.ID, code, "verify-idem-key", "verify device")
	result2 := verifyLogin(t, svc, ctx, challenge.ID, code, "verify-idem-key", "verify device")

	if result1.Session.ID != result2.Session.ID {
		t.Fatalf("expected cached session ID %s, got %s", result1.Session.ID, result2.Session.ID)
	}
	if result1.Device.ID != result2.Device.ID {
		t.Fatalf("expected cached device ID %s, got %s", result1.Device.ID, result2.Device.ID)
	}
	if result1.Tokens.AccessToken != result2.Tokens.AccessToken {
		t.Fatalf("expected cached access token")
	}
}

func TestRegisterDeviceDeduplicatesSuccessfulRequestsByIdempotencyKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 15, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account := createAccount(t, svc, ctx, "register-idem-user", "register-idem@example.com")
	challenge, targets := beginLogin(t, svc, ctx, account.Username, "")
	code := sender.codeFor(targets[0].DestinationMask)
	loginResult := verifyLogin(t, svc, ctx, challenge.ID, code, "", "register device")

	device1, session1, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "desktop",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "desktop-key-1",
		IdempotencyKey: "register-idem-key",
	})
	if err != nil {
		t.Fatalf("register device first call: %v", err)
	}

	device2, session2, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "desktop",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "desktop-key-1",
		IdempotencyKey: "register-idem-key",
	})
	if err != nil {
		t.Fatalf("register device second call: %v", err)
	}

	if device1.ID != device2.ID {
		t.Fatalf("expected cached device ID %s, got %s", device1.ID, device2.ID)
	}
	if session1.ID != session2.ID {
		t.Fatalf("expected cached session ID %s, got %s", session1.ID, session2.ID)
	}

	devices, err := svc.ListDevices(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected two devices after deduped register, got %d", len(devices))
	}
}

func TestRegisterDeviceConflictsOnDifferentRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 17, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account := createAccount(t, svc, ctx, "register-conflict-user", "register-conflict@example.com")
	challenge, targets := beginLogin(t, svc, ctx, account.Username, "")
	code := sender.codeFor(targets[0].DestinationMask)
	loginResult := verifyLogin(t, svc, ctx, challenge.ID, code, "", "register conflict login")

	_, _, err = svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "desktop",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "register-conflict-key-1",
		IdempotencyKey: "register-conflict-key",
	})
	if err != nil {
		t.Fatalf("first register device: %v", err)
	}

	_, _, err = svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "tablet",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "register-conflict-key-2",
		IdempotencyKey: "register-conflict-key",
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict on mismatched replay, got %v", err)
	}
}

func TestRegisterDeviceRollsBackDeviceWhenSessionUpdateFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &failingUpdateSessionStore{Store: baseStore}
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 20, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account := createAccount(t, svc, ctx, "register-rollback-user", "register-rollback@example.com")
	challenge, targets := beginLogin(t, svc, ctx, account.Username, "")
	code := sender.codeFor(targets[0].DestinationMask)
	loginResult := verifyLogin(t, svc, ctx, challenge.ID, code, "", "register rollback login")

	_, _, err = svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "tablet",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "tablet-key-1",
		IdempotencyKey: "register-rollback-key",
	})
	if err == nil {
		t.Fatalf("expected first register device call to fail")
	}

	devices, err := baseStore.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices after failed register: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected one device after rollback, got %d", len(devices))
	}

	_, _, err = svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "tablet",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "tablet-key-2",
		IdempotencyKey: "register-rollback-key",
	})
	if err != nil {
		t.Fatalf("expected retry to succeed after rollback, got %v", err)
	}

	devices, err = baseStore.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices after retry: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected two devices after retry, got %d", len(devices))
	}
}

func TestRegisterDeviceRejectsSuspendedAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &countingAuthStore{Store: baseStore}
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 25, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account := createAccount(t, svc, ctx, "register-suspended-user", "register-suspended@example.com")
	challenge, targets := beginLogin(t, svc, ctx, account.Username, "")
	code := sender.codeFor(targets[0].DestinationMask)
	loginResult := verifyLogin(t, svc, ctx, challenge.ID, code, "", "register suspended login")

	storedAccount, err := baseStore.AccountByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load account before suspension: %v", err)
	}
	storedAccount.Status = identity.AccountStatusSuspended
	if _, err := baseStore.SaveAccount(ctx, storedAccount); err != nil {
		t.Fatalf("suspend account: %v", err)
	}

	saveLoginChallengeCalls, saveDeviceCalls, saveSessionCalls, saveAccountCalls, lockAccountCalls, sessionByIDCalls, accountByIDCalls, updateSessionCalls := store.counts()

	_, _, err = svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "tablet",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "tablet-key",
		IdempotencyKey: "register-suspended-key",
	})
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for suspended account, got %v", err)
	}

	afterSaveLoginChallengeCalls, afterSaveDeviceCalls, afterSaveSessionCalls, afterSaveAccountCalls, afterLockAccountCalls, afterSessionByIDCalls, afterAccountByIDCalls, afterUpdateSessionCalls := store.counts()

	if afterSaveLoginChallengeCalls != saveLoginChallengeCalls {
		t.Fatalf("expected no login challenge writes, got %d -> %d", saveLoginChallengeCalls, afterSaveLoginChallengeCalls)
	}
	if afterSaveDeviceCalls != saveDeviceCalls {
		t.Fatalf("expected no device writes, got %d -> %d", saveDeviceCalls, afterSaveDeviceCalls)
	}
	if afterSaveSessionCalls != saveSessionCalls {
		t.Fatalf("expected no session writes, got %d -> %d", saveSessionCalls, afterSaveSessionCalls)
	}
	if afterSaveAccountCalls != saveAccountCalls {
		t.Fatalf("expected no account writes, got %d -> %d", saveAccountCalls, afterSaveAccountCalls)
	}
	if afterLockAccountCalls != lockAccountCalls+1 {
		t.Fatalf("expected one account lock for register device, got %d -> %d", lockAccountCalls, afterLockAccountCalls)
	}
	if afterSessionByIDCalls != sessionByIDCalls+1 {
		t.Fatalf("expected one session lookup for register device, got %d -> %d", sessionByIDCalls, afterSessionByIDCalls)
	}
	if afterAccountByIDCalls != accountByIDCalls+1 {
		t.Fatalf("expected one account lookup for register device, got %d -> %d", accountByIDCalls, afterAccountByIDCalls)
	}
	if afterUpdateSessionCalls != updateSessionCalls {
		t.Fatalf("expected no session updates, got %d -> %d", updateSessionCalls, afterUpdateSessionCalls)
	}
}
