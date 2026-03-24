package pgstore

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

func TestDeleteDeviceReturnsConflictOnForeignKeyViolation(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		`DELETE FROM %s WHERE id = $1`,
		qualifiedName("tenant", "identity_devices"),
	))).
		WithArgs("dev-1").
		WillReturnError(&pgconn.PgError{Code: "23503"})

	if err := store.DeleteDevice(context.Background(), "dev-1"); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSaveDeviceRejectsCrossAccountSession(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		`SELECT %s FROM %s WHERE id = $1`,
		sessionColumnList,
		qualifiedName("tenant", "identity_sessions"),
	))).
		WithArgs("sess-1").
		WillReturnRows(sqlmock.NewRows(strings.Split(sessionColumnList, ", ")).AddRow(
			"sess-1",
			"acc-peer",
			"dev-peer",
			"Peer device",
			identity.DevicePlatformWeb,
			"",
			"",
			identity.SessionStatusActive,
			true,
			time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
			time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
			nil,
		))

	_, err := store.SaveDevice(context.Background(), identity.Device{
		ID:         "dev-1",
		AccountID:  "acc-1",
		SessionID:  "sess-1",
		Name:       "Alice phone",
		Platform:   identity.DevicePlatformWeb,
		Status:     identity.DeviceStatusActive,
		PublicKey:  "pk-1",
		CreatedAt:  time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
		LastSeenAt: time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestDeleteDeviceRejectsDeviceDeletionWhenSessionHasPeers(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, time.March, 24, 0, 15, 0, 0, time.UTC)

	account, err := store.SaveAccount(ctx, identity.Account{
		ID:          "acc-1",
		Kind:        identity.AccountKindUser,
		Username:    "alice",
		DisplayName: "Alice",
		Status:      identity.AccountStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save account: %v", err)
	}

	const primarySessionID = "sess-primary"
	const primaryDeviceID = "dev-primary"
	const peerSessionID = "sess-peer"
	const peerDeviceID = "dev-peer"

	if err := store.WithinTx(ctx, func(tx identity.Store) error {
		if _, err := tx.SaveDevice(ctx, identity.Device{
			ID:         primaryDeviceID,
			AccountID:  account.ID,
			SessionID:  primarySessionID,
			Name:       "Primary device",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-primary",
			CreatedAt:  now,
			LastSeenAt: now,
		}); err != nil {
			return err
		}
		if _, err := tx.SaveSession(ctx, identity.Session{
			ID:             primarySessionID,
			AccountID:      account.ID,
			DeviceID:       primaryDeviceID,
			DeviceName:     "Primary device",
			DevicePlatform: identity.DevicePlatformWeb,
			Status:         identity.SessionStatusActive,
			Current:        true,
			CreatedAt:      now,
			LastSeenAt:     now,
		}); err != nil {
			return err
		}
		if _, err := tx.SaveDevice(ctx, identity.Device{
			ID:         peerDeviceID,
			AccountID:  account.ID,
			SessionID:  peerSessionID,
			Name:       "Peer device",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-peer",
			CreatedAt:  now,
			LastSeenAt: now,
		}); err != nil {
			return err
		}
		_, err := tx.SaveSession(ctx, identity.Session{
			ID:             peerSessionID,
			AccountID:      account.ID,
			DeviceID:       peerDeviceID,
			DeviceName:     "Peer device",
			DevicePlatform: identity.DevicePlatformWeb,
			Status:         identity.SessionStatusActive,
			Current:        false,
			CreatedAt:      now,
			LastSeenAt:     now,
		})
		return err
	}); err != nil {
		t.Fatalf("seed device/session graph: %v", err)
	}

	if err := store.DeleteDevice(ctx, primaryDeviceID); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	if err := store.WithinTx(ctx, func(tx identity.Store) error {
		return tx.DeleteDevice(ctx, primaryDeviceID)
	}); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected transactional conflict, got %v", err)
	}
}
