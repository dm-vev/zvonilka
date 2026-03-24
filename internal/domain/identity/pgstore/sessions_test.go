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

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

func TestDeleteSessionReturnsConflictOnForeignKeyViolation(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		`DELETE FROM %s WHERE id = $1`,
		qualifiedName("tenant", "identity_sessions"),
	))).
		WithArgs("sess-1").
		WillReturnError(&pgconn.PgError{Code: "23503"})

	if err := store.DeleteSession(context.Background(), "sess-1"); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestWithinTxMapsCommitForeignKeyViolationToConflict(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		`DELETE FROM %s WHERE id = $1`,
		qualifiedName("tenant", "identity_sessions"),
	))).
		WithArgs("sess-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(&pgconn.PgError{Code: "23503"})

	err := store.WithinTx(context.Background(), func(tx identity.Store) error {
		return tx.DeleteSession(context.Background(), "sess-1")
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSaveSessionRejectsCrossAccountDevice(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		`SELECT %s FROM %s WHERE id = $1`,
		deviceColumnList,
		qualifiedName("tenant", "identity_devices"),
	))).
		WithArgs("dev-1").
		WillReturnRows(sqlmock.NewRows(strings.Split(deviceColumnList, ", ")).AddRow(
			"dev-1",
			"acc-peer",
			"sess-peer",
			"Peer device",
			identity.DevicePlatformWeb,
			identity.DeviceStatusActive,
			"pk-peer",
			"",
			time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
			time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
			nil,
			nil,
		))

	_, err := store.SaveSession(context.Background(), identity.Session{
		ID:             "sess-1",
		AccountID:      "acc-1",
		DeviceID:       "dev-1",
		DeviceName:     "Alice laptop",
		DevicePlatform: identity.DevicePlatformWeb,
		Status:         identity.SessionStatusActive,
		Current:        true,
		CreatedAt:      time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
		LastSeenAt:     time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
	})
	if !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpdateSessionMapsDeferredForeignKeyViolationToNotFound(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		`SELECT %s FROM %s WHERE id = $1`,
		deviceColumnList,
		qualifiedName("tenant", "identity_devices"),
	))).
		WithArgs("dev-missing").
		WillReturnRows(sqlmock.NewRows(strings.Split(deviceColumnList, ", ")))

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`
UPDATE %s
SET account_id = $2,
    device_id = $3,
    device_name = $4,
    device_platform = $5,
    ip_address = $6,
    user_agent = $7,
    status = $8,
    current = $9,
    last_seen_at = $10,
    revoked_at = $11
WHERE id = $1
RETURNING %s
`, qualifiedName("tenant", "identity_sessions"), sessionColumnList))).
		WithArgs(
			"sess-1",
			"acc-1",
			"dev-missing",
			"Alice laptop",
			identity.DevicePlatformWeb,
			"",
			"",
			identity.SessionStatusActive,
			true,
			time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
			nil,
		).
		WillReturnError(&pgconn.PgError{Code: "23503"})

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		`SELECT %s FROM %s WHERE id = $1`,
		accountColumnList,
		qualifiedName("tenant", "identity_accounts"),
	))).
		WithArgs("acc-1").
		WillReturnRows(sqlmock.NewRows(strings.Split(accountColumnList, ", ")).AddRow(
			"acc-1",
			identity.AccountKindUser,
			"alice",
			"Alice",
			"",
			"",
			"",
			"[]",
			identity.AccountStatusActive,
			"",
			"",
			time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
			time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
			nil,
			nil,
			"",
		))

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		`SELECT %s FROM %s WHERE id = $1`,
		deviceColumnList,
		qualifiedName("tenant", "identity_devices"),
	))).
		WithArgs("dev-missing").
		WillReturnRows(sqlmock.NewRows(strings.Split(deviceColumnList, ", ")))

	_, err := store.UpdateSession(context.Background(), identity.Session{
		ID:             "sess-1",
		AccountID:      "acc-1",
		DeviceID:       "dev-missing",
		DeviceName:     "Alice laptop",
		DevicePlatform: identity.DevicePlatformWeb,
		Status:         identity.SessionStatusActive,
		Current:        true,
		CreatedAt:      time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
		LastSeenAt:     time.Date(2026, time.March, 24, 0, 30, 0, 0, time.UTC),
	})
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
