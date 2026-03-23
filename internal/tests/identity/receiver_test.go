package identity_test

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
)

func TestNilStoreMethodsReturnInvalidInput(t *testing.T) {
	t.Parallel()

	var store *pgstore.Store
	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "within tx",
			fn: func() error {
				return store.WithinTx(ctx, func(identity.Store) error { return nil })
			},
		},
		{
			name: "save account",
			fn: func() error {
				_, err := store.SaveAccount(ctx, identity.Account{ID: "acc-1", CreatedAt: now, UpdatedAt: now})
				return err
			},
		},
		{
			name: "save join request",
			fn: func() error {
				_, err := store.SaveJoinRequest(ctx, identity.JoinRequest{ID: "join-1", RequestedAt: now, ExpiresAt: now.Add(time.Hour)})
				return err
			},
		},
		{
			name: "save login challenge",
			fn: func() error {
				_, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{ID: "chal-1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
				return err
			},
		},
		{
			name: "save device",
			fn: func() error {
				_, err := store.SaveDevice(ctx, identity.Device{ID: "dev-1", CreatedAt: now, LastSeenAt: now})
				return err
			},
		},
		{
			name: "save session",
			fn: func() error {
				_, err := store.SaveSession(ctx, identity.Session{ID: "sess-1", CreatedAt: now, LastSeenAt: now})
				return err
			},
		},
		{
			name: "account by id",
			fn: func() error {
				_, err := store.AccountByID(ctx, "acc-1")
				return err
			},
		},
		{
			name: "join request by id",
			fn: func() error {
				_, err := store.JoinRequestByID(ctx, "join-1")
				return err
			},
		},
		{
			name: "login challenge by id",
			fn: func() error {
				_, err := store.LoginChallengeByID(ctx, "chal-1")
				return err
			},
		},
		{
			name: "device by id",
			fn: func() error {
				_, err := store.DeviceByID(ctx, "dev-1")
				return err
			},
		},
		{
			name: "session by id",
			fn: func() error {
				_, err := store.SessionByID(ctx, "sess-1")
				return err
			},
		},
		{
			name: "delete account",
			fn: func() error {
				return store.DeleteAccount(ctx, "acc-1")
			},
		},
		{
			name: "delete login challenge",
			fn: func() error {
				return store.DeleteLoginChallenge(ctx, "chal-1")
			},
		},
		{
			name: "delete device",
			fn: func() error {
				return store.DeleteDevice(ctx, "dev-1")
			},
		},
		{
			name: "delete session",
			fn: func() error {
				return store.DeleteSession(ctx, "sess-1")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				err := tc.fn()
				require.ErrorIs(t, err, identity.ErrInvalidInput)
			})
		})
	}
}

func TestNilContextMethodsReturnInvalidInput(t *testing.T) {
	t.Parallel()

	db, _, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	store, err := pgstore.New(db, "tenant")
	require.NoError(t, err)

	var nilContext context.Context
	now := time.Date(2026, time.March, 24, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "within tx",
			fn: func() error {
				return store.WithinTx(nilContext, func(identity.Store) error { return nil })
			},
		},
		{
			name: "save account",
			fn: func() error {
				_, err := store.SaveAccount(nilContext, identity.Account{ID: "acc-1", CreatedAt: now, UpdatedAt: now})
				return err
			},
		},
		{
			name: "join request by id",
			fn: func() error {
				_, err := store.JoinRequestByID(nilContext, "join-1")
				return err
			},
		},
		{
			name: "save session",
			fn: func() error {
				_, err := store.SaveSession(nilContext, identity.Session{ID: "sess-1", CreatedAt: now, LastSeenAt: now})
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NotPanics(t, func() {
				err := tc.fn()
				require.ErrorIs(t, err, identity.ErrInvalidInput)
			})
		})
	}
}
