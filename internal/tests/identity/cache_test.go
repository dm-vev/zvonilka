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

// countingAuthStore tracks how often auth-side persistence calls are executed.
type countingAuthStore struct {
	identity.Store

	mu                 sync.Mutex
	saveLoginChallenge int
	saveDevice         int
	saveSession        int
	saveAccount        int
	sessionByID        int
	accountByID        int
	updateSession      int
}

// WithinTx preserves call counting while executing the callback against the transactional store.
func (s *countingAuthStore) WithinTx(ctx context.Context, fn func(identity.Store) error) error {
	return s.Store.WithinTx(ctx, func(tx identity.Store) error {
		return fn(&countingAuthTxStore{
			Store:  tx,
			parent: s,
		})
	})
}

// SaveLoginChallenge counts and forwards login-challenge writes.
func (s *countingAuthStore) SaveLoginChallenge(
	ctx context.Context,
	challenge identity.LoginChallenge,
) (identity.LoginChallenge, error) {
	s.mu.Lock()
	s.saveLoginChallenge++
	s.mu.Unlock()

	return s.Store.SaveLoginChallenge(ctx, challenge)
}

// SaveDevice counts and forwards device writes.
func (s *countingAuthStore) SaveDevice(
	ctx context.Context,
	device identity.Device,
) (identity.Device, error) {
	s.mu.Lock()
	s.saveDevice++
	s.mu.Unlock()

	return s.Store.SaveDevice(ctx, device)
}

// SaveSession counts and forwards session writes.
func (s *countingAuthStore) SaveSession(
	ctx context.Context,
	session identity.Session,
) (identity.Session, error) {
	s.mu.Lock()
	s.saveSession++
	s.mu.Unlock()

	return s.Store.SaveSession(ctx, session)
}

// SaveAccount counts and forwards account writes.
func (s *countingAuthStore) SaveAccount(
	ctx context.Context,
	account identity.Account,
) (identity.Account, error) {
	s.mu.Lock()
	s.saveAccount++
	s.mu.Unlock()

	return s.Store.SaveAccount(ctx, account)
}

// SessionByID counts and forwards session lookups by ID.
func (s *countingAuthStore) SessionByID(ctx context.Context, sessionID string) (identity.Session, error) {
	s.mu.Lock()
	s.sessionByID++
	s.mu.Unlock()

	return s.Store.SessionByID(ctx, sessionID)
}

// AccountByID counts and forwards account lookups by ID.
func (s *countingAuthStore) AccountByID(ctx context.Context, accountID string) (identity.Account, error) {
	s.mu.Lock()
	s.accountByID++
	s.mu.Unlock()

	return s.Store.AccountByID(ctx, accountID)
}

// UpdateSession counts and forwards session updates.
func (s *countingAuthStore) UpdateSession(
	ctx context.Context,
	session identity.Session,
) (identity.Session, error) {
	s.mu.Lock()
	s.updateSession++
	s.mu.Unlock()

	return s.Store.UpdateSession(ctx, session)
}

// counts returns the observed call totals for each tracked method.
func (s *countingAuthStore) counts() (saveLoginChallenge, saveDevice, saveSession, saveAccount, sessionByID, accountByID, updateSession int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveLoginChallenge, s.saveDevice, s.saveSession, s.saveAccount, s.sessionByID, s.accountByID, s.updateSession
}

type countingAuthTxStore struct {
	identity.Store

	parent *countingAuthStore
}

func (s *countingAuthTxStore) SaveLoginChallenge(
	ctx context.Context,
	challenge identity.LoginChallenge,
) (identity.LoginChallenge, error) {
	s.parent.mu.Lock()
	s.parent.saveLoginChallenge++
	s.parent.mu.Unlock()

	return s.Store.SaveLoginChallenge(ctx, challenge)
}

func (s *countingAuthTxStore) SaveDevice(
	ctx context.Context,
	device identity.Device,
) (identity.Device, error) {
	s.parent.mu.Lock()
	s.parent.saveDevice++
	s.parent.mu.Unlock()

	return s.Store.SaveDevice(ctx, device)
}

func (s *countingAuthTxStore) SaveSession(
	ctx context.Context,
	session identity.Session,
) (identity.Session, error) {
	s.parent.mu.Lock()
	s.parent.saveSession++
	s.parent.mu.Unlock()

	return s.Store.SaveSession(ctx, session)
}

func (s *countingAuthTxStore) SaveAccount(
	ctx context.Context,
	account identity.Account,
) (identity.Account, error) {
	s.parent.mu.Lock()
	s.parent.saveAccount++
	s.parent.mu.Unlock()

	return s.Store.SaveAccount(ctx, account)
}

func (s *countingAuthTxStore) SessionByID(ctx context.Context, sessionID string) (identity.Session, error) {
	s.parent.mu.Lock()
	s.parent.sessionByID++
	s.parent.mu.Unlock()

	return s.Store.SessionByID(ctx, sessionID)
}

func (s *countingAuthTxStore) AccountByID(ctx context.Context, accountID string) (identity.Account, error) {
	s.parent.mu.Lock()
	s.parent.accountByID++
	s.parent.mu.Unlock()

	return s.Store.AccountByID(ctx, accountID)
}

func (s *countingAuthTxStore) UpdateSession(
	ctx context.Context,
	session identity.Session,
) (identity.Session, error) {
	s.parent.mu.Lock()
	s.parent.updateSession++
	s.parent.mu.Unlock()

	return s.Store.UpdateSession(ctx, session)
}

func TestBeginLoginIdempotencyKeyDeduplicatesSuccessfulChallenge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &countingAuthStore{Store: baseStore}
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 21, 30, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "begin-login-user",
		DisplayName: "Begin Login User",
		Email:       "begin-login@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	firstChallenge, firstTargets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       "begin-login-user",
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-login-key",
	})
	if err != nil {
		t.Fatalf("first begin login: %v", err)
	}
	if len(firstTargets) != 1 {
		t.Fatalf("expected one login target, got %d", len(firstTargets))
	}

	secondChallenge, secondTargets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       "begin-login-user",
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-login-key",
	})
	if err != nil {
		t.Fatalf("second begin login: %v", err)
	}
	if secondChallenge.ID != firstChallenge.ID {
		t.Fatalf("expected cached challenge %s, got %s", firstChallenge.ID, secondChallenge.ID)
	}
	if len(secondTargets) != len(firstTargets) {
		t.Fatalf("expected cached targets length %d, got %d", len(firstTargets), len(secondTargets))
	}
	if sender.totalSends() != 1 {
		t.Fatalf("expected one code send, got %d", sender.totalSends())
	}

	saveLoginChallenge, _, _, _, _, _, _ := store.counts()
	if saveLoginChallenge != 1 {
		t.Fatalf("expected one SaveLoginChallenge call, got %d", saveLoginChallenge)
	}
}

func TestVerifyLoginCodeIdempotencyKeyDeduplicatesSuccessfulLogin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &countingAuthStore{Store: baseStore}
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 22, 0, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "verify-login-user",
		DisplayName: "Verify Login User",
		Email:       "verify-login@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username: "verify-login-user",
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

	first, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     "Verify Device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "verify-key-1",
		IdempotencyKey: "verify-login-key",
	})
	if err != nil {
		t.Fatalf("first verify login: %v", err)
	}

	second, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     "Verify Device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "verify-key-1",
		IdempotencyKey: "verify-login-key",
	})
	if err != nil {
		t.Fatalf("second verify login: %v", err)
	}
	if second.Session.ID != first.Session.ID {
		t.Fatalf("expected cached session %s, got %s", first.Session.ID, second.Session.ID)
	}
	if second.Device.ID != first.Device.ID {
		t.Fatalf("expected cached device %s, got %s", first.Device.ID, second.Device.ID)
	}

	saveLoginChallenge, saveDevice, saveSession, saveAccount, sessionByID, accountByID, updateSession := store.counts()
	if saveLoginChallenge != 2 {
		t.Fatalf("expected two SaveLoginChallenge calls for begin+verify, got %d", saveLoginChallenge)
	}
	if saveDevice != 1 {
		t.Fatalf("expected one SaveDevice call, got %d", saveDevice)
	}
	if saveSession != 1 {
		t.Fatalf("expected one SaveSession call, got %d", saveSession)
	}
	if saveAccount != 2 {
		t.Fatalf("expected two SaveAccount calls, got %d", saveAccount)
	}
	if sessionByID != 0 {
		t.Fatalf("expected cached verify to avoid SessionByID calls, got %d", sessionByID)
	}
	if accountByID != 1 {
		t.Fatalf("expected one AccountByID call, got %d", accountByID)
	}
	if updateSession != 0 {
		t.Fatalf("expected verify cache to avoid UpdateSession calls, got %d", updateSession)
	}
}

func TestRegisterDeviceIdempotencyKeyDeduplicatesSuccessfulRegistration(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &countingAuthStore{Store: baseStore}
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 22, 30, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "register-device-user",
		DisplayName: "Register Device User",
		Email:       "register-device@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username: "register-device-user",
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

	loginResult, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        code,
		DeviceName:  "Register Device Phone",
		Platform:    identity.DevicePlatformIOS,
		PublicKey:   "register-device-key",
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	firstDevice, firstSession, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "Register Device Mac",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "register-device-extra-key",
		IdempotencyKey: "register-device-key",
	})
	if err != nil {
		t.Fatalf("first register device: %v", err)
	}

	secondDevice, secondSession, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "Register Device Mac",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "register-device-extra-key",
		IdempotencyKey: "register-device-key",
	})
	if err != nil {
		t.Fatalf("second register device: %v", err)
	}
	if secondDevice.ID != firstDevice.ID {
		t.Fatalf("expected cached device %s, got %s", firstDevice.ID, secondDevice.ID)
	}
	if secondSession.ID != firstSession.ID {
		t.Fatalf("expected cached session %s, got %s", firstSession.ID, secondSession.ID)
	}

	saveLoginChallenge, saveDevice, saveSession, saveAccount, sessionByID, accountByID, updateSession := store.counts()
	if saveLoginChallenge != 2 {
		t.Fatalf("expected two SaveLoginChallenge calls for begin+verify, got %d", saveLoginChallenge)
	}
	if saveDevice != 2 {
		t.Fatalf("expected two SaveDevice calls for login device + registered device, got %d", saveDevice)
	}
	if saveSession != 1 {
		t.Fatalf("expected one SaveSession call for login session, got %d", saveSession)
	}
	if saveAccount != 3 {
		t.Fatalf("expected three SaveAccount calls, got %d", saveAccount)
	}
	if sessionByID != 2 {
		t.Fatalf("expected two SessionByID calls, got %d", sessionByID)
	}
	if accountByID != 2 {
		t.Fatalf("expected two AccountByID calls, got %d", accountByID)
	}
	if updateSession != 1 {
		t.Fatalf("expected one UpdateSession call, got %d", updateSession)
	}
}

func TestBeginLoginIdempotencyKeyConflictsOnDifferentRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &countingAuthStore{Store: baseStore}
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 24, 0, 0, 0, 0, time.UTC)

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, _, err = svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "begin-login-conflict-user",
		DisplayName: "Begin Login Conflict User",
		Email:       "begin-login-conflict@example.com",
		Phone:       "+1 555 0101",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	_, _, err = svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       "begin-login-conflict-user",
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-login-conflict-key",
	})
	if err != nil {
		t.Fatalf("first begin login: %v", err)
	}

	_, _, err = svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       "begin-login-conflict-user",
		Delivery:       identity.LoginDeliveryChannelSMS,
		IdempotencyKey: "begin-login-conflict-key",
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict on mismatched replay, got %v", err)
	}
}
