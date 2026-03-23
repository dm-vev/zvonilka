package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	teststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
)

func TestRevokeSessionIdempotencyKeyDeduplicatesSuccessfulRevoke(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &countingSessionStore{Store: baseStore}
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 23, 21, 0, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "revoke-session-user",
		DisplayName: "Revoke Session User",
		Email:       "revoke-session@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	session := newLoggedInAccount(t, svc, sender, account.Username, "session-phone", "session-key")

	first, err := svc.RevokeSession(ctx, identity.RevokeSessionParams{
		SessionID:      session.ID,
		Reason:         "logout",
		IdempotencyKey: "revoke-session-key",
	})
	if err != nil {
		t.Fatalf("first revoke session: %v", err)
	}
	if first.Status != identity.SessionStatusRevoked {
		t.Fatalf("expected revoked session, got %s", first.Status)
	}

	second, err := svc.RevokeSession(ctx, identity.RevokeSessionParams{
		SessionID:      session.ID,
		Reason:         "logout",
		IdempotencyKey: "revoke-session-key",
	})
	if err != nil {
		t.Fatalf("second revoke session: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("expected cached session %s, got %s", first.ID, second.ID)
	}
	if second.RevokedAt != first.RevokedAt {
		t.Fatalf("expected cached revoked timestamp %v, got %v", first.RevokedAt, second.RevokedAt)
	}

	sessionByIDCalls, _, updateSessionCalls := store.counts()
	if sessionByIDCalls != 1 {
		t.Fatalf("expected one SessionByID call, got %d", sessionByIDCalls)
	}
	if updateSessionCalls != 1 {
		t.Fatalf("expected one UpdateSession call, got %d", updateSessionCalls)
	}
}

func TestRevokeAllSessionsIdempotencyKeyDeduplicatesSuccessfulBulkRevoke(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &countingSessionStore{Store: baseStore}
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 23, 21, 30, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "bulk-revoke-user",
		DisplayName: "Bulk Revoke User",
		Email:       "bulk-revoke@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	firstSession := newLoggedInAccount(t, svc, sender, account.Username, "bulk-phone-1", "bulk-key-1")
	secondSession := newLoggedInAccount(t, svc, sender, account.Username, "bulk-phone-2", "bulk-key-2")
	_, baselineSessionsByAccountIDCalls, baselineUpdateSessionCalls := store.counts()

	firstRevoked, err := svc.RevokeAllSessions(ctx, account.ID, identity.RevokeAllSessionsParams{
		Reason:         "logout all",
		IdempotencyKey: "revoke-all-key",
	})
	if err != nil {
		t.Fatalf("first revoke all sessions: %v", err)
	}
	if firstRevoked != 2 {
		t.Fatalf("expected to revoke 2 sessions, got %d", firstRevoked)
	}

	secondRevoked, err := svc.RevokeAllSessions(ctx, account.ID, identity.RevokeAllSessionsParams{
		Reason:         "logout all",
		IdempotencyKey: "revoke-all-key",
	})
	if err != nil {
		t.Fatalf("second revoke all sessions: %v", err)
	}
	if secondRevoked != firstRevoked {
		t.Fatalf("expected cached revoke count %d, got %d", firstRevoked, secondRevoked)
	}

	_, sessionsByAccountIDCalls, updateSessionCalls := store.counts()
	if sessionsByAccountIDCalls-baselineSessionsByAccountIDCalls != 1 {
		t.Fatalf(
			"expected one SessionsByAccountID call during revoke, got %d",
			sessionsByAccountIDCalls-baselineSessionsByAccountIDCalls,
		)
	}
	if updateSessionCalls-baselineUpdateSessionCalls != 2 {
		t.Fatalf(
			"expected two UpdateSession calls during revoke, got %d",
			updateSessionCalls-baselineUpdateSessionCalls,
		)
	}

	sessions, err := svc.ListSessions(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	for _, session := range sessions {
		if session.Current {
			t.Fatalf("expected no current sessions after revocation, found %s", session.ID)
		}
	}
	if firstSession.ID == secondSession.ID {
		t.Fatalf("expected distinct sessions")
	}
}

func TestRevokeAllSessionsReturnsZeroCountOnRollback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	baseStore := teststore.NewMemoryStore()
	store := &failingUpdateSessionStore{Store: baseStore, failOnCall: 2}
	sender := &recordingCodeSender{}
	clock := &steppedClock{
		now:  time.Date(2026, time.March, 23, 22, 0, 0, 0, time.UTC),
		step: time.Minute,
	}

	svc, err := identity.NewService(store, sender, identity.WithNow(clock.Now))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	account, _, err := svc.CreateAccount(ctx, identity.CreateAccountParams{
		Username:    "revoke-rollback-user",
		DisplayName: "Revoke Rollback User",
		Email:       "revoke-rollback@example.com",
		AccountKind: identity.AccountKindUser,
		CreatedBy:   "admin-1",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	firstSession := newLoggedInAccount(t, svc, sender, account.Username, "revoke-rollback-phone-1", "revoke-rollback-key-1")
	secondSession := newLoggedInAccount(t, svc, sender, account.Username, "revoke-rollback-phone-2", "revoke-rollback-key-2")

	revoked, err := svc.RevokeAllSessions(ctx, account.ID, identity.RevokeAllSessionsParams{
		Reason:         "logout all",
		IdempotencyKey: "revoke-rollback-key",
	})
	if err == nil {
		t.Fatalf("expected revoke all sessions to fail")
	}
	if revoked != 0 {
		t.Fatalf("expected zero revoked count on rollback, got %d", revoked)
	}

	sessions, err := svc.ListSessions(ctx, account.ID)
	if err != nil {
		t.Fatalf("list sessions after rollback: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions after rollback, got %d", len(sessions))
	}

	currentCount := 0
	for _, session := range sessions {
		if session.Status != identity.SessionStatusActive {
			t.Fatalf("expected active session after rollback, got %s", session.Status)
		}
		if session.Current {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected one current session after rollback, got %d", currentCount)
	}
	if firstSession.ID == secondSession.ID {
		t.Fatalf("expected distinct sessions")
	}
}
