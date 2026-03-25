package presence_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	identityteststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	teststore "github.com/dm-vev/zvonilka/internal/domain/presence/teststore"
)

func TestGetPresenceDerivesOnlineFromRecentActivity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 15, 0, 0, 0, time.UTC)
	identityStore := identityteststore.NewMemoryStore()
	if _, err := identityStore.SaveAccount(ctx, identity.Account{
		ID:         "acc-1",
		Username:   "alice",
		Status:     identity.AccountStatusActive,
		LastAuthAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("save account: %v", err)
	}
	if _, err := identityStore.SaveSession(ctx, identity.Session{
		ID:         "sess-1",
		AccountID:  "acc-1",
		LastSeenAt: now.Add(-90 * time.Second),
		Status:     identity.SessionStatusActive,
		Current:    true,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if _, err := identityStore.SaveDevice(ctx, identity.Device{
		ID:         "dev-1",
		AccountID:  "acc-1",
		SessionID:  "sess-1",
		LastSeenAt: now.Add(-time.Minute),
		Status:     identity.DeviceStatusActive,
	}); err != nil {
		t.Fatalf("save device: %v", err)
	}

	store := teststore.NewMemoryStore()
	svc, err := presence.NewService(store, identityStore, presence.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	snapshot, err := svc.GetPresence(ctx, presence.GetParams{AccountID: "acc-1"})
	if err != nil {
		t.Fatalf("get presence: %v", err)
	}
	if snapshot.State != presence.PresenceStateOnline {
		t.Fatalf("expected online state, got %s", snapshot.State)
	}
	if snapshot.LastSeenHidden {
		t.Fatal("expected last seen to be visible")
	}
	if snapshot.LastSeenAt.IsZero() {
		t.Fatal("expected last seen timestamp")
	}
}

func TestSetPresenceHidesLastSeenForOtherViewers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 15, 30, 0, 0, time.UTC)
	identityStore := identityteststore.NewMemoryStore()
	if _, err := identityStore.SaveAccount(ctx, identity.Account{
		ID:         "acc-1",
		Username:   "alice",
		Status:     identity.AccountStatusActive,
		LastAuthAt: now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("save account: %v", err)
	}
	if _, err := identityStore.SaveSession(ctx, identity.Session{
		ID:         "sess-1",
		AccountID:  "acc-1",
		LastSeenAt: now.Add(-90 * time.Second),
		Status:     identity.SessionStatusActive,
		Current:    true,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	store := teststore.NewMemoryStore()
	svc, err := presence.NewService(store, identityStore, presence.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	saved, err := svc.SetPresence(ctx, presence.SetParams{
		AccountID:       "acc-1",
		State:           presence.PresenceStateBusy,
		CustomStatus:    "In a call",
		HideLastSeenFor: 15 * time.Minute,
		RecordedAt:      now,
	})
	if err != nil {
		t.Fatalf("set presence: %v", err)
	}
	if saved.State != presence.PresenceStateBusy {
		t.Fatalf("expected busy state, got %s", saved.State)
	}
	if saved.HiddenUntil.IsZero() {
		t.Fatal("expected hidden until timestamp")
	}

	other, err := svc.GetPresence(ctx, presence.GetParams{
		AccountID:       "acc-1",
		ViewerAccountID: "acc-2",
	})
	if err != nil {
		t.Fatalf("get presence for other viewer: %v", err)
	}
	if other.State != presence.PresenceStateBusy {
		t.Fatalf("expected busy state, got %s", other.State)
	}
	if !other.LastSeenHidden {
		t.Fatal("expected last seen to be hidden for other viewers")
	}
	if !other.LastSeenAt.IsZero() {
		t.Fatalf("expected hidden last seen to be zero, got %v", other.LastSeenAt)
	}

	self, err := svc.GetPresence(ctx, presence.GetParams{
		AccountID:       "acc-1",
		ViewerAccountID: "acc-1",
	})
	if err != nil {
		t.Fatalf("get presence for self: %v", err)
	}
	if self.LastSeenHidden {
		t.Fatal("expected last seen to stay visible to self")
	}
	if self.LastSeenAt.IsZero() {
		t.Fatal("expected self to see last seen timestamp")
	}
}

func TestSetPresenceRejectsUnknownAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, err := presence.NewService(teststore.NewMemoryStore(), identityteststore.NewMemoryStore())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.SetPresence(ctx, presence.SetParams{
		AccountID:  "missing",
		State:      presence.PresenceStateOnline,
		RecordedAt: time.Now().UTC(),
	})
	if !errors.Is(err, presence.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestGetPresenceRejectsInactiveAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identityteststore.NewMemoryStore()
	for _, account := range []identity.Account{
		{
			ID:       "suspended",
			Username: "suspended",
			Status:   identity.AccountStatusSuspended,
		},
		{
			ID:       "revoked",
			Username: "revoked",
			Status:   identity.AccountStatusRevoked,
		},
	} {
		if _, err := identityStore.SaveAccount(ctx, account); err != nil {
			t.Fatalf("save account %s: %v", account.ID, err)
		}
	}

	svc, err := presence.NewService(teststore.NewMemoryStore(), identityStore)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, accountID := range []string{"suspended", "revoked"} {
		_, err := svc.GetPresence(ctx, presence.GetParams{AccountID: accountID})
		if !errors.Is(err, presence.ErrNotFound) {
			t.Fatalf("expected not found for %s, got %v", accountID, err)
		}
	}
}

func TestSetPresenceRejectsInactiveAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identityteststore.NewMemoryStore()
	for _, account := range []identity.Account{
		{
			ID:       "suspended",
			Username: "suspended",
			Status:   identity.AccountStatusSuspended,
		},
		{
			ID:       "revoked",
			Username: "revoked",
			Status:   identity.AccountStatusRevoked,
		},
	} {
		if _, err := identityStore.SaveAccount(ctx, account); err != nil {
			t.Fatalf("save account %s: %v", account.ID, err)
		}
	}

	svc, err := presence.NewService(teststore.NewMemoryStore(), identityStore)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, accountID := range []string{"suspended", "revoked"} {
		_, err := svc.SetPresence(ctx, presence.SetParams{
			AccountID:  accountID,
			State:      presence.PresenceStateOnline,
			RecordedAt: time.Now().UTC(),
		})
		if !errors.Is(err, presence.ErrNotFound) {
			t.Fatalf("expected not found for %s, got %v", accountID, err)
		}
	}
}

func TestGetPresencePreservesExplicitOfflineState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 17, 0, 0, 0, time.UTC)
	identityStore := identityteststore.NewMemoryStore()
	if _, err := identityStore.SaveAccount(ctx, identity.Account{
		ID:         "acc-1",
		Username:   "alice",
		Status:     identity.AccountStatusActive,
		LastAuthAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("save account: %v", err)
	}
	if _, err := identityStore.SaveSession(ctx, identity.Session{
		ID:         "sess-1",
		AccountID:  "acc-1",
		LastSeenAt: now.Add(-time.Minute),
		Status:     identity.SessionStatusActive,
		Current:    true,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}

	svc, err := presence.NewService(teststore.NewMemoryStore(), identityStore, presence.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.SetPresence(ctx, presence.SetParams{
		AccountID:  "acc-1",
		State:      presence.PresenceStateOffline,
		RecordedAt: now,
	})
	if err != nil {
		t.Fatalf("set presence: %v", err)
	}

	snapshot, err := svc.GetPresence(ctx, presence.GetParams{
		AccountID:       "acc-1",
		ViewerAccountID: "viewer",
	})
	if err != nil {
		t.Fatalf("get presence: %v", err)
	}
	if snapshot.State != presence.PresenceStateOffline {
		t.Fatalf("expected explicit offline state, got %s", snapshot.State)
	}
	if snapshot.LastSeenHidden {
		t.Fatal("expected last seen to remain visible")
	}
	if snapshot.LastSeenAt.IsZero() {
		t.Fatal("expected last seen to remain visible")
	}
}

func TestGetPresenceIgnoresInactiveSessionAndDeviceActivity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 16, 0, 0, 0, time.UTC)
	identityStore := identityteststore.NewMemoryStore()
	if _, err := identityStore.SaveAccount(ctx, identity.Account{
		ID:         "acc-1",
		Username:   "alice",
		Status:     identity.AccountStatusActive,
		LastAuthAt: now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("save account: %v", err)
	}
	if _, err := identityStore.SaveSession(ctx, identity.Session{
		ID:         "sess-active",
		AccountID:  "acc-1",
		LastSeenAt: now.Add(-20 * time.Minute),
		Status:     identity.SessionStatusActive,
		Current:    true,
	}); err != nil {
		t.Fatalf("save session: %v", err)
	}
	if _, err := identityStore.SaveSession(ctx, identity.Session{
		ID:         "sess-revoked",
		AccountID:  "acc-1",
		LastSeenAt: now.Add(-1 * time.Minute),
		Status:     identity.SessionStatusRevoked,
	}); err != nil {
		t.Fatalf("save revoked session: %v", err)
	}
	if _, err := identityStore.SaveDevice(ctx, identity.Device{
		ID:         "dev-active",
		AccountID:  "acc-1",
		SessionID:  "sess-active",
		LastSeenAt: now.Add(-25 * time.Minute),
		Status:     identity.DeviceStatusActive,
	}); err != nil {
		t.Fatalf("save device: %v", err)
	}
	if _, err := identityStore.SaveDevice(ctx, identity.Device{
		ID:         "dev-revoked",
		AccountID:  "acc-1",
		SessionID:  "sess-revoked",
		LastSeenAt: now.Add(-1 * time.Minute),
		Status:     identity.DeviceStatusRevoked,
	}); err != nil {
		t.Fatalf("save revoked device: %v", err)
	}

	svc, err := presence.NewService(teststore.NewMemoryStore(), identityStore, presence.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	snapshot, err := svc.GetPresence(ctx, presence.GetParams{AccountID: "acc-1"})
	if err != nil {
		t.Fatalf("get presence: %v", err)
	}
	if snapshot.State != presence.PresenceStateOffline {
		t.Fatalf("expected offline state, got %s", snapshot.State)
	}
	if snapshot.LastSeenHidden {
		t.Fatal("expected last seen to remain visible")
	}
	if !snapshot.LastSeenAt.Equal(now.Add(-20 * time.Minute)) {
		t.Fatalf("expected only active activity to count, got %v", snapshot.LastSeenAt)
	}
}

func TestPresenceRejectsSuspendedAccount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	identityStore := identityteststore.NewMemoryStore()
	if _, err := identityStore.SaveAccount(ctx, identity.Account{
		ID:       "acc-1",
		Username: "alice",
		Status:   identity.AccountStatusSuspended,
	}); err != nil {
		t.Fatalf("save account: %v", err)
	}

	svc, err := presence.NewService(teststore.NewMemoryStore(), identityStore)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.GetPresence(ctx, presence.GetParams{AccountID: "acc-1"})
	if !errors.Is(err, presence.ErrNotFound) {
		t.Fatalf("expected not found for suspended account, got %v", err)
	}

	_, err = svc.SetPresence(ctx, presence.SetParams{
		AccountID:  "acc-1",
		State:      presence.PresenceStateOnline,
		RecordedAt: time.Now().UTC(),
	})
	if !errors.Is(err, presence.ErrNotFound) {
		t.Fatalf("expected not found on update for suspended account, got %v", err)
	}
}

func TestListPresenceSkipsInactiveAccounts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 17, 0, 0, 0, time.UTC)
	identityStore := identityteststore.NewMemoryStore()
	for _, account := range []identity.Account{
		{
			ID:         "acc-active",
			Username:   "alice",
			Status:     identity.AccountStatusActive,
			LastAuthAt: now.Add(-2 * time.Minute),
		},
		{
			ID:       "acc-suspended",
			Username: "bob",
			Status:   identity.AccountStatusSuspended,
		},
	} {
		if _, err := identityStore.SaveAccount(ctx, account); err != nil {
			t.Fatalf("save account %s: %v", account.ID, err)
		}
	}

	svc, err := presence.NewService(teststore.NewMemoryStore(), identityStore, presence.WithNow(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	snapshots, err := svc.ListPresence(ctx, []string{"acc-active", "acc-suspended"}, "viewer")
	if err != nil {
		t.Fatalf("list presence: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected only active account to remain, got %d", len(snapshots))
	}
	if _, ok := snapshots["acc-active"]; !ok {
		t.Fatal("expected active account snapshot")
	}
	if _, ok := snapshots["acc-suspended"]; ok {
		t.Fatal("did not expect suspended account snapshot")
	}
}
