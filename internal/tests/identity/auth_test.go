package identity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestBeginLoginIdempotencyReturnsCachedChallenge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 19, 30, 0, 0, time.UTC))

	account := createUserAccount(t, svc, ctx, "begin-idempotency")

	firstChallenge, firstTargets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       account.Username,
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-login-key",
	})
	if err != nil {
		t.Fatalf("begin login: %v", err)
	}
	if len(firstTargets) != 1 {
		t.Fatalf("expected one login target, got %d", len(firstTargets))
	}

	secondChallenge, secondTargets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       account.Username,
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: "begin-login-key",
	})
	if err != nil {
		t.Fatalf("repeat begin login: %v", err)
	}
	if firstChallenge.ID != secondChallenge.ID {
		t.Fatalf("expected cached challenge %s, got %s", firstChallenge.ID, secondChallenge.ID)
	}
	if len(secondTargets) != 1 || secondTargets[0] != firstTargets[0] {
		t.Fatalf("expected cached login targets")
	}
	if sender.totalSends() != 1 {
		t.Fatalf("expected one code send, got %d", sender.totalSends())
	}
}

func TestVerifyLoginCodeRollsBackOnAccountSaveFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &failingSaveAccountStore{Store: baseStore}
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 19, 45, 0, 0, time.UTC))

	account := createUserAccount(t, svc, ctx, "rollback-account")
	challenge, code := startEmailLogin(t, svc, sender, ctx, account.Username, "begin-rollback")

	_, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     "Rollback Device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "rollback-key",
		IdempotencyKey: "verify-rollback",
	})
	if err == nil {
		t.Fatalf("expected verify login to fail")
	}
	if errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected downstream failure, got conflict")
	}

	devices, err := baseStore.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices after failed verify: %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("expected no devices after failed verify, got %d", len(devices))
	}

	sessions, err := baseStore.SessionsByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions after failed verify: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions after failed verify, got %d", len(sessions))
	}

	storedAccount, err := baseStore.AccountByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load account after failed verify: %v", err)
	}
	if !storedAccount.LastAuthAt.IsZero() {
		t.Fatalf("expected LastAuthAt to remain zero, got %s", storedAccount.LastAuthAt)
	}

	storedChallenge, err := baseStore.LoginChallengeByID(ctx, challenge.ID)
	if err != nil {
		t.Fatalf("load login challenge after failed verify: %v", err)
	}
	if storedChallenge.Used {
		t.Fatalf("expected login challenge to remain unused after rollback")
	}
	if !storedChallenge.UsedAt.IsZero() {
		t.Fatalf("expected login challenge UsedAt to be cleared, got %s", storedChallenge.UsedAt)
	}

	retryResult, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     "Rollback Device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "rollback-key",
		IdempotencyKey: "verify-rollback",
	})
	if err != nil {
		t.Fatalf("expected replay-safe retry after rollback, got %v", err)
	}
	if retryResult.Session.ID == "" || retryResult.Device.ID == "" {
		t.Fatalf("expected replay-safe retry to issue session and device")
	}
	if retryResult.Session.ID == "" {
		t.Fatalf("expected session on retry")
	}
	if retryResult.Device.ID == "" {
		t.Fatalf("expected device on retry")
	}

	cachedResult, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     "Rollback Device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "rollback-key",
		IdempotencyKey: "verify-rollback",
	})
	if err != nil {
		t.Fatalf("expected cached retry to succeed, got %v", err)
	}
	if cachedResult.Session.ID != retryResult.Session.ID {
		t.Fatalf("expected cached session %s, got %s", retryResult.Session.ID, cachedResult.Session.ID)
	}
}

func TestVerifyLoginCodeRestoresPreviousCurrentSessionOnLateFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &failingSaveAccountStore{
		Store:      baseStore,
		failOnCall: 3,
	}
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 19, 55, 0, 0, time.UTC))

	account := createUserAccount(t, svc, ctx, "restore-current-session")
	firstLogin := loginUserResult(t, svc, sender, ctx, account.Username, "begin-restore-current-1", "verify-restore-current-1")

	challenge, code := startEmailLogin(t, svc, sender, ctx, account.Username, "begin-restore-current-2")
	_, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        code,
		DeviceName:  "restore-current-session-device-2",
		Platform:    identity.DevicePlatformIOS,
		PublicKey:   "restore-current-session-public-key-2",
	})
	if err == nil {
		t.Fatalf("expected verify login to fail")
	}
	if errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected downstream failure, got conflict")
	}

	sessions, err := baseStore.SessionsByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions after failed verify: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one persisted session after rollback, got %d", len(sessions))
	}
	if sessions[0].ID != firstLogin.Session.ID {
		t.Fatalf("expected original session %s to remain, got %s", firstLogin.Session.ID, sessions[0].ID)
	}
	if !sessions[0].Current {
		t.Fatalf("expected original session to remain current")
	}

	devices, err := baseStore.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices after failed verify: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected one persisted device after rollback, got %d", len(devices))
	}
	if devices[0].SessionID != firstLogin.Session.ID {
		t.Fatalf("expected device to remain attached to original session %s, got %s", firstLogin.Session.ID, devices[0].SessionID)
	}
}

func TestVerifyLoginCodeIdempotencyReturnsCachedLoginResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 20, 0, 0, 0, time.UTC))

	account := createUserAccount(t, svc, ctx, "verify-idempotency")
	challenge, code := startEmailLogin(t, svc, sender, ctx, account.Username, "begin-verify-idempotency")

	firstResult, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     "Verify Device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "verify-public-key",
		IdempotencyKey: "verify-login-key",
	})
	if err != nil {
		t.Fatalf("verify login: %v", err)
	}

	secondResult, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     "Verify Device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      "verify-public-key",
		IdempotencyKey: "verify-login-key",
	})
	if err != nil {
		t.Fatalf("repeat verify login: %v", err)
	}

	if firstResult.Session.ID != secondResult.Session.ID {
		t.Fatalf("expected cached session %s, got %s", firstResult.Session.ID, secondResult.Session.ID)
	}
	if firstResult.Device.ID != secondResult.Device.ID {
		t.Fatalf("expected cached device %s, got %s", firstResult.Device.ID, secondResult.Device.ID)
	}
	if firstResult.Tokens != secondResult.Tokens {
		t.Fatalf("expected cached tokens")
	}

	devices, err := store.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices after cached verify: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected one device after cached verify, got %d", len(devices))
	}
}

func TestRegisterDeviceIdempotencyReturnsCachedDeviceAndSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	svc := newReliabilityService(t, store, sender, time.Date(2026, time.March, 23, 20, 15, 0, 0, time.UTC))

	account := createUserAccount(t, svc, ctx, "register-idempotency")
	loginResult := loginUserResult(t, svc, sender, ctx, account.Username, "begin-register-idempotency", "verify-register-idempotency")

	firstDevice, firstSession, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "Register Device",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "register-device-key",
		PushToken:      "push-1",
		IdempotencyKey: "register-device-key",
	})
	if err != nil {
		t.Fatalf("register device: %v", err)
	}

	secondDevice, secondSession, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
		SessionID:      loginResult.Session.ID,
		DeviceName:     "Register Device",
		Platform:       identity.DevicePlatformDesktop,
		PublicKey:      "register-device-key",
		PushToken:      "push-1",
		IdempotencyKey: "register-device-key",
	})
	if err != nil {
		t.Fatalf("repeat register device: %v", err)
	}

	if firstDevice.ID != secondDevice.ID {
		t.Fatalf("expected cached device %s, got %s", firstDevice.ID, secondDevice.ID)
	}
	if firstSession.ID != secondSession.ID {
		t.Fatalf("expected cached session %s, got %s", firstSession.ID, secondSession.ID)
	}

	devices, err := store.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		t.Fatalf("list devices after cached register: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected two devices after cached register, got %d", len(devices))
	}
}

// loginUser performs the full login flow and returns both the challenge and the resulting session bundle.
func loginUser(
	t *testing.T,
	svc *identity.Service,
	sender *recordingCodeSender,
	ctx context.Context,
	username string,
	beginKey string,
	verifyKey string,
) (identity.LoginChallenge, identity.LoginResult) {
	t.Helper()

	challenge, code := startEmailLogin(t, svc, sender, ctx, username, beginKey)
	result, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     username + "-device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      username + "-public-key",
		IdempotencyKey: verifyKey,
	})
	if err != nil {
		t.Fatalf("verify login for %s: %v", username, err)
	}

	return challenge, result
}

func TestLoginIssuesSingleCurrentSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 30, 0, 0, time.UTC)
	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account := createUserAccount(t, svc, ctx, "current-session")
	firstLogin := loginUserResult(t, svc, sender, ctx, account.Username, "begin-current-1", "verify-current-1")
	now = now.Add(time.Minute)
	secondLogin := loginUserResult(t, svc, sender, ctx, account.Username, "begin-current-2", "verify-current-2")

	if firstLogin.Session.ID == secondLogin.Session.ID {
		t.Fatalf("expected a new session for the second login")
	}

	sessions, err := svc.ListSessions(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected two sessions, got %d", len(sessions))
	}

	currentCount := 0
	for _, session := range sessions {
		if session.Current {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected one current session, got %d", currentCount)
	}

	for _, session := range sessions {
		if session.ID == firstLogin.Session.ID && session.Current {
			t.Fatalf("expected first session to stop being current")
		}
		if session.ID == secondLogin.Session.ID && !session.Current {
			t.Fatalf("expected second session to be current")
		}
	}
}

func TestVerifyLoginCodeUpdatesLastAuthAt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	now := time.Date(2026, time.March, 23, 20, 45, 0, 0, time.UTC)
	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account := createUserAccount(t, svc, ctx, "last-auth-at")
	now = now.Add(3 * time.Minute)
	_, _ = loginUser(t, svc, sender, ctx, account.Username, "begin-last-auth", "verify-last-auth")

	storedAccount, err := store.AccountByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("load account after login: %v", err)
	}
	if !storedAccount.LastAuthAt.Equal(now) {
		t.Fatalf("expected LastAuthAt to be updated to %s, got %s", now, storedAccount.LastAuthAt)
	}
	if !storedAccount.UpdatedAt.Equal(now) {
		t.Fatalf("expected UpdatedAt to be updated to %s, got %s", now, storedAccount.UpdatedAt)
	}
}

// newReliabilityService constructs a service with a deterministic clock for reliability tests.
func newReliabilityService(
	t *testing.T,
	store identity.Store,
	sender *recordingCodeSender,
	fixedNow time.Time,
) *identity.Service {
	t.Helper()

	svc, err := identity.NewService(store, sender, identity.WithNow(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return svc
}

// createUserAccount creates a minimal user account for reliability flows.
func createUserAccount(
	t *testing.T,
	svc *identity.Service,
	ctx context.Context,
	username string,
) identity.Account {
	t.Helper()

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    username,
		DisplayName: username,
		Email:       username + "@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account %s: %v", username, err)
	}

	return account
}

// startEmailLogin starts an email-based login challenge and returns the code captured by the fake sender.
func startEmailLogin(
	t *testing.T,
	svc *identity.Service,
	sender *recordingCodeSender,
	ctx context.Context,
	username string,
	idempotencyKey string,
) (identity.LoginChallenge, string) {
	t.Helper()

	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username:       username,
		Delivery:       identity.LoginDeliveryChannelEmail,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("begin login for %s: %v", username, err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one login target, got %d", len(targets))
	}

	code := sender.codeFor(targets[0].DestinationMask)
	if code == "" {
		t.Fatalf("expected recorded login code")
	}

	return challenge, code
}

// loginUserResult performs a complete login and returns the issued result bundle.
func loginUserResult(
	t *testing.T,
	svc *identity.Service,
	sender *recordingCodeSender,
	ctx context.Context,
	username string,
	beginKey string,
	verifyKey string,
) identity.LoginResult {
	t.Helper()

	challenge, code := startEmailLogin(t, svc, sender, ctx, username, beginKey)
	result, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID:    challenge.ID,
		Code:           code,
		DeviceName:     username + "-device",
		Platform:       identity.DevicePlatformIOS,
		PublicKey:      username + "-public-key",
		IdempotencyKey: verifyKey,
	})
	if err != nil {
		t.Fatalf("verify login for %s: %v", username, err)
	}

	return result
}
