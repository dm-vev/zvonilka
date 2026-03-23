package identity_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

// countingSessionStore tracks read and write operations used by the session read model.
type countingSessionStore struct {
	identity.Store

	mu                       sync.Mutex
	sessionByIDCalls         int
	sessionsByAccountIDCalls int
	updateSessionCalls       int
}

// WithinTx preserves read-model counting while executing the callback against a transactional store.
func (s *countingSessionStore) WithinTx(ctx context.Context, fn func(identity.Store) error) error {
	return s.Store.WithinTx(ctx, func(tx identity.Store) error {
		return fn(&countingSessionTxStore{
			Store:  tx,
			parent: s,
		})
	})
}

// SessionByID counts and forwards single-session lookups.
func (s *countingSessionStore) SessionByID(ctx context.Context, sessionID string) (identity.Session, error) {
	s.mu.Lock()
	s.sessionByIDCalls++
	s.mu.Unlock()

	return s.Store.SessionByID(ctx, sessionID)
}

// SessionsByAccountID counts and forwards account session listings.
func (s *countingSessionStore) SessionsByAccountID(
	ctx context.Context,
	accountID string,
) ([]identity.Session, error) {
	s.mu.Lock()
	s.sessionsByAccountIDCalls++
	s.mu.Unlock()

	return s.Store.SessionsByAccountID(ctx, accountID)
}

// UpdateSession counts and forwards session updates.
func (s *countingSessionStore) UpdateSession(ctx context.Context, session identity.Session) (identity.Session, error) {
	s.mu.Lock()
	s.updateSessionCalls++
	s.mu.Unlock()

	return s.Store.UpdateSession(ctx, session)
}

// counts returns the observed session-store call totals.
func (s *countingSessionStore) counts() (sessionByID, sessionsByAccountID, updateSession int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.sessionByIDCalls, s.sessionsByAccountIDCalls, s.updateSessionCalls
}

type countingSessionTxStore struct {
	identity.Store

	parent *countingSessionStore
}

func (s *countingSessionTxStore) SessionByID(ctx context.Context, sessionID string) (identity.Session, error) {
	s.parent.mu.Lock()
	s.parent.sessionByIDCalls++
	s.parent.mu.Unlock()

	return s.Store.SessionByID(ctx, sessionID)
}

func (s *countingSessionTxStore) SessionsByAccountID(
	ctx context.Context,
	accountID string,
) ([]identity.Session, error) {
	s.parent.mu.Lock()
	s.parent.sessionsByAccountIDCalls++
	s.parent.mu.Unlock()

	return s.Store.SessionsByAccountID(ctx, accountID)
}

func (s *countingSessionTxStore) UpdateSession(ctx context.Context, session identity.Session) (identity.Session, error) {
	s.parent.mu.Lock()
	s.parent.updateSessionCalls++
	s.parent.mu.Unlock()

	return s.Store.UpdateSession(ctx, session)
}

// steppedClock returns monotonically increasing timestamps on each call.
type steppedClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

// Now returns the next scheduled clock value.
func (c *steppedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	current := c.now
	if c.step == 0 {
		c.step = time.Minute
	}
	c.now = c.now.Add(c.step)

	return current
}

// newLoggedInAccount performs a full login and returns the resulting session.
func newLoggedInAccount(
	t *testing.T,
	svc *identity.Service,
	sender *recordingCodeSender,
	username string,
	deviceName string,
	publicKey string,
) identity.Session {
	t.Helper()

	ctx := context.Background()
	challenge, targets, err := svc.BeginLogin(ctx, identity.BeginLoginParams{
		Username: username,
		Delivery: identity.LoginDeliveryChannelEmail,
	})
	if err != nil {
		t.Fatalf("begin login for %s: %v", username, err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected one login target, got %d", len(targets))
	}

	code := sender.codeFor(targets[0].DestinationMask)
	if code == "" {
		t.Fatalf("expected recorded login code for %s", username)
	}

	loginResult, err := svc.VerifyLoginCode(ctx, identity.VerifyLoginCodeParams{
		ChallengeID: challenge.ID,
		Code:        code,
		DeviceName:  deviceName,
		Platform:    identity.DevicePlatformIOS,
		PublicKey:   publicKey,
	})
	if err != nil {
		t.Fatalf("verify login for %s: %v", username, err)
	}

	return loginResult.Session
}

func TestListSessionsExposesOnlyOneCurrentSessionAfterMultipleLogins(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 23, 20, 0, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(baseStore, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "current-session-user",
		DisplayName: "Current Session User",
		Email:       "current-session@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	firstSession := newLoggedInAccount(t, svc, sender, account.Username, "phone-1", "public-key-1")
	secondSession := newLoggedInAccount(t, svc, sender, account.Username, "phone-2", "public-key-2")

	sessions, err := svc.ListSessions(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	currentCount := 0
	currentSessionID := ""
	for _, session := range sessions {
		if !session.Current {
			continue
		}
		currentCount++
		currentSessionID = session.ID
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one current session, got %d", currentCount)
	}
	if currentSessionID != secondSession.ID {
		t.Fatalf("expected newest session %s to be current, got %s", secondSession.ID, currentSessionID)
	}
	if firstSession.ID == secondSession.ID {
		t.Fatalf("expected distinct sessions")
	}
}

func TestVerifyLoginCodeDoesNotRewriteCurrentSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &countingSessionStore{Store: baseStore}
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 23, 20, 30, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "no-rewrite-current-session",
		DisplayName: "No Rewrite Current Session",
		Email:       "no-rewrite-current-session@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	firstSession := newLoggedInAccount(t, svc, sender, account.Username, "phone-1", "public-key-1")
	secondSession := newLoggedInAccount(t, svc, sender, account.Username, "phone-2", "public-key-2")

	_, _, updateSessionCalls := store.counts()
	if updateSessionCalls != 0 {
		t.Fatalf("expected no UpdateSession calls during login, got %d", updateSessionCalls)
	}

	sessions, err := svc.ListSessions(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	currentCount := 0
	currentSessionID := ""
	for _, session := range sessions {
		if !session.Current {
			continue
		}
		currentCount++
		currentSessionID = session.ID
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one current session, got %d", currentCount)
	}
	if currentSessionID != secondSession.ID {
		t.Fatalf("expected newest session %s to be current, got %s", secondSession.ID, currentSessionID)
	}
	if firstSession.ID == secondSession.ID {
		t.Fatalf("expected distinct sessions")
	}

	_, _, updateSessionCalls = store.counts()
	if updateSessionCalls != 0 {
		t.Fatalf("expected no UpdateSession calls after listing sessions, got %d", updateSessionCalls)
	}
}
