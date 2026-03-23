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

// accountGateStore lets the test pause the shared account boundary while other
// store methods continue to interleave, which makes the revoke race observable.
type accountGateStore struct {
	identity.Store

	stateMu     sync.Mutex
	lock        sync.Mutex
	blocking    bool
	enteredOnce sync.Once
	entered     chan struct{}
	release     chan struct{}
}

// WithinTx keeps transactional calls interleavable so the test can reproduce the race.
func (s *accountGateStore) WithinTx(ctx context.Context, fn func(identity.Store) error) error {
	tx := &accountGateTxStore{
		Store:  s.Store,
		parent: s,
	}
	defer func() {
		if tx.holdsAccountLock {
			s.lock.Unlock()
		}
	}()

	return fn(tx)
}

// SaveAccount is only used outside the race harness, so it forwards directly.
func (s *accountGateStore) SaveAccount(ctx context.Context, account identity.Account) (identity.Account, error) {
	return s.Store.SaveAccount(ctx, account)
}

// enableBlocking turns on the SaveAccount gate after setup work is complete.
func (s *accountGateStore) enableBlocking() {
	s.stateMu.Lock()
	s.blocking = true
	s.stateMu.Unlock()
}

// accountGateTxStore serializes SaveAccount calls on the shared account boundary.
type accountGateTxStore struct {
	identity.Store

	parent *accountGateStore

	holdsAccountLock bool
}

// SaveAccount acquires the shared account boundary for the duration of the transaction.
func (s *accountGateTxStore) SaveAccount(ctx context.Context, account identity.Account) (identity.Account, error) {
	s.parent.lock.Lock()
	s.holdsAccountLock = true

	s.parent.stateMu.Lock()
	blocking := s.parent.blocking
	s.parent.stateMu.Unlock()
	if blocking {
		s.parent.enteredOnce.Do(func() {
			close(s.parent.entered)
		})
		<-s.parent.release
	}

	saved, err := s.Store.SaveAccount(ctx, account)
	return saved, err
}

// newAccountGateStore wraps a store with a controllable SaveAccount gate.
func newAccountGateStore(store identity.Store) *accountGateStore {
	return &accountGateStore{
		Store:   store,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func TestRevokeAllSessionsBlocksConcurrentLoginUntilAccountBoundaryReleases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := newAccountGateStore(baseStore)
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 24, 2, 0, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "revoke-login-race-user",
		DisplayName: "Revoke Login Race User",
		Email:       "revoke-login-race@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	signedIn := newLoggedInAccount(t, svc, sender, account.Username, "phone-1", "login-key-1")
	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username: account.Username,
		Delivery: identity.LoginDeliveryChannelEmail,
	})
	if err != nil {
		t.Fatalf("begin login for race: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one login target, got %d", len(targets))
	}
	code := sender.codeFor(targets[0].DestinationMask)
	if code == "" {
		t.Fatalf("expected recorded login code")
	}

	store.enableBlocking()

	revokeDone := make(chan error, 1)
	go func() {
		_, err := svc.RevokeAllSessions(ctx, account.ID, identity.RevokeAllSessionsParams{
			Reason:         "revoke all",
			IdempotencyKey: "revoke-login-race-key",
		})
		revokeDone <- err
	}()

	<-store.entered

	loginDone := make(chan error, 1)
	go func() {
		_, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
			ChallengeID: challenge.ID,
			Code:        code,
			DeviceName:  "race laptop",
			Platform:    identity.DevicePlatformDesktop,
			PublicKey:   "race-login-key",
		})
		loginDone <- err
	}()

	select {
	case err := <-loginDone:
		t.Fatalf("login completed while revoke was blocked: %v", err)
	case <-time.After(250 * time.Millisecond):
	}

	close(store.release)

	if err := <-revokeDone; err != nil {
		t.Fatalf("revoke all sessions: %v", err)
	}
	if err := <-loginDone; err != nil {
		t.Fatalf("verify login after revoke: %v", err)
	}

	sessions, err := svc.ListSessions(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions after race: %v", err)
	}
	currentCount := 0
	activeCount := 0
	for _, session := range sessions {
		if session.Status == identity.SessionStatusActive {
			activeCount++
		}
		if session.Current {
			currentCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected one active session after revoke/login serialization, got %d", activeCount)
	}
	if currentCount != 1 {
		t.Fatalf("expected one current session after revoke/login serialization, got %d", currentCount)
	}
	if signedIn.ID == "" {
		t.Fatalf("expected preexisting session")
	}
}

func TestRevokeAllSessionsBlocksConcurrentRegisterDeviceUntilAccountBoundaryReleases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := newAccountGateStore(baseStore)
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 24, 2, 30, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "revoke-register-race-user",
		DisplayName: "Revoke Register Race User",
		Email:       "revoke-register-race@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	session := newLoggedInAccount(t, svc, sender, account.Username, "phone-1", "register-key-1")

	store.enableBlocking()

	revokeDone := make(chan error, 1)
	go func() {
		_, err := svc.RevokeAllSessions(ctx, account.ID, identity.RevokeAllSessionsParams{
			Reason:         "revoke all",
			IdempotencyKey: "revoke-register-race-key",
		})
		revokeDone <- err
	}()

	<-store.entered

	registerDone := make(chan error, 1)
	go func() {
		_, _, err := svc.RegisterDevice(ctx, identity.RegisterDeviceParams{
			SessionID:      session.ID,
			DeviceName:     "race tablet",
			Platform:       identity.DevicePlatformDesktop,
			PublicKey:      "race-register-key",
			IdempotencyKey: "revoke-register-race-register-key",
		})
		registerDone <- err
	}()

	select {
	case err := <-registerDone:
		t.Fatalf("register device completed while revoke was blocked: %v", err)
	case <-time.After(250 * time.Millisecond):
	}

	close(store.release)

	if err := <-revokeDone; err != nil {
		t.Fatalf("revoke all sessions: %v", err)
	}

	err = <-registerDone
	if !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("expected register device to fail after revoke, got %v", err)
	}

	sessions, err := svc.ListSessions(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions after race: %v", err)
	}
	for _, current := range sessions {
		if current.Current {
			t.Fatalf("expected no current session after revoke/register race, found %s", current.ID)
		}
	}
}
