package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return store, mock, db
}

func TestWithinTxCommitMarkerCommits(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectBegin()
	mock.ExpectCommit()

	err := store.WithinTx(context.Background(), func(identity.Store) error {
		return domainstorage.Commit(errors.New("boom"))
	})
	if err == nil {
		t.Fatal("expected commit-marked error")
	}
	if err.Error() != "boom" {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestWithinTxRollsBackOnError(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectBegin()
	mock.ExpectRollback()

	err := store.WithinTx(context.Background(), func(identity.Store) error {
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected rollback error")
	}
	if err.Error() != "boom" {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestWithinTxMapsCommitConstraintViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "foreign key",
			err:  &pgconn.PgError{Code: "23503"},
			want: identity.ErrConflict,
		},
		{
			name: "check",
			err:  &pgconn.PgError{Code: "23514"},
			want: identity.ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store, mock, _ := newMockStore(t)
			mock.ExpectBegin()
			mock.ExpectCommit().WillReturnError(tt.err)

			err := store.WithinTx(context.Background(), func(identity.Store) error {
				return nil
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("expectations: %v", err)
			}
		})
	}
}

func TestSaveAccountPersistsRow(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)
	now := time.Date(2026, time.March, 23, 22, 30, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE id = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("acc-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE username = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE email = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("alice@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE phone = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("12345").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`
INSERT INTO %s (
	id, kind, username, display_name, bio, email, phone, roles, status, bot_token_hash,
	created_by, created_at, updated_at, disabled_at, last_auth_at, custom_badge_emoji
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
	$11, $12, $13, $14, $15, $16
)
ON CONFLICT (id) DO UPDATE SET
	kind = EXCLUDED.kind,
	username = EXCLUDED.username,
	display_name = EXCLUDED.display_name,
	bio = EXCLUDED.bio,
	email = EXCLUDED.email,
	phone = EXCLUDED.phone,
	roles = EXCLUDED.roles,
	status = EXCLUDED.status,
	bot_token_hash = EXCLUDED.bot_token_hash,
	created_by = EXCLUDED.created_by,
	updated_at = EXCLUDED.updated_at,
	disabled_at = EXCLUDED.disabled_at,
	last_auth_at = EXCLUDED.last_auth_at,
	custom_badge_emoji = EXCLUDED.custom_badge_emoji
RETURNING %s
`, qualifiedName("tenant", "identity_accounts"), accountColumnList))).
		WithArgs(
			"acc-1",
			identity.AccountKindUser,
			"alice",
			"Alice",
			"",
			"alice@example.com",
			"12345",
			`["admin"]`,
			identity.AccountStatusActive,
			"",
			"admin-1",
			now,
			now,
			nil,
			nil,
			"",
		).
		WillReturnRows(sqlmock.NewRows(strings.Split(accountColumnList, ", ")).AddRow(
			"acc-1",
			identity.AccountKindUser,
			"alice",
			"Alice",
			"",
			"alice@example.com",
			"12345",
			`["admin"]`,
			identity.AccountStatusActive,
			"",
			"admin-1",
			now,
			now,
			nil,
			nil,
			"",
		))
	mock.ExpectCommit()

	account, err := store.SaveAccount(context.Background(), identity.Account{
		ID:          "acc-1",
		Kind:        identity.AccountKindUser,
		Username:    "alice",
		DisplayName: "Alice",
		Email:       "alice@example.com",
		Phone:       "12345",
		Roles:       []identity.Role{"admin"},
		Status:      identity.AccountStatusActive,
		CreatedBy:   "admin-1",
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save account: %v", err)
	}
	if account.ID != "acc-1" || account.Username != "alice" {
		t.Fatalf("unexpected saved account: %+v", account)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSaveLoginChallengeReturnsInvalidInputOnCheckViolation(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)
	now := time.Date(2026, time.March, 23, 22, 30, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`
INSERT INTO %s (
	id, account_id, account_kind, code_hash, delivery_channel, targets, expires_at, created_at, used_at, used
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
	account_id = EXCLUDED.account_id,
	account_kind = EXCLUDED.account_kind,
	code_hash = EXCLUDED.code_hash,
	delivery_channel = EXCLUDED.delivery_channel,
	targets = EXCLUDED.targets,
	expires_at = EXCLUDED.expires_at,
	used_at = EXCLUDED.used_at,
	used = EXCLUDED.used
RETURNING %s
`, qualifiedName("tenant", "identity_login_challenges"), loginChallengeColumnList))).
		WithArgs(
			"chal-1",
			"acc-1",
			identity.AccountKindUser,
			"hash",
			identity.LoginDeliveryChannel("fax"),
			"[]",
			now.Add(time.Hour),
			now,
			nil,
			false,
		).
		WillReturnError(&pgconn.PgError{Code: "23514"})

	_, err := store.SaveLoginChallenge(context.Background(), identity.LoginChallenge{
		ID:              "chal-1",
		AccountID:       "acc-1",
		AccountKind:     identity.AccountKindUser,
		CodeHash:        "hash",
		DeliveryChannel: identity.LoginDeliveryChannel("fax"),
		ExpiresAt:       now.Add(time.Hour),
		CreatedAt:       now,
	})
	if !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestLockAccountLocksExistingRow(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		`SELECT id FROM %s WHERE id = $1 FOR UPDATE`,
		qualifiedName("tenant", "identity_accounts"),
	))).
		WithArgs("acc-1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("acc-1"))
	mock.ExpectCommit()

	if err := store.LockAccount(context.Background(), "acc-1"); err != nil {
		t.Fatalf("lock account: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestLockAccountReturnsNotFound(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		`SELECT id FROM %s WHERE id = $1 FOR UPDATE`,
		qualifiedName("tenant", "identity_accounts"),
	))).
		WithArgs("acc-1").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	if err := store.LockAccount(context.Background(), "acc-1"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSaveJoinRequestExpiresStalePendingRequest(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)
	now := time.Date(2026, time.March, 23, 23, 0, 0, 0, time.UTC)
	requestedAt := now.Add(-4 * time.Hour)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE username = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("alice").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE email = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("alice@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE phone = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("12345").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(`
UPDATE %s
SET status = $2,
    reviewed_at = CURRENT_TIMESTAMP,
    reviewed_by = '',
    decision_reason = 'join request expired'
WHERE status = $1
  AND expires_at <= CURRENT_TIMESTAMP
  AND (username = $3 OR email = $4 OR phone = $5)
`, qualifiedName("tenant", "identity_join_requests")))).
		WithArgs(
			identity.JoinRequestStatusPending,
			identity.JoinRequestStatusExpired,
			"alice",
			"alice@example.com",
			"12345",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`
INSERT INTO %s (
	id, username, display_name, email, phone, note, status, requested_at, reviewed_at, reviewed_by, decision_reason, expires_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
	username = EXCLUDED.username,
	display_name = EXCLUDED.display_name,
	email = EXCLUDED.email,
	phone = EXCLUDED.phone,
	note = EXCLUDED.note,
	status = EXCLUDED.status,
	reviewed_at = EXCLUDED.reviewed_at,
	reviewed_by = EXCLUDED.reviewed_by,
	decision_reason = EXCLUDED.decision_reason,
	expires_at = EXCLUDED.expires_at
RETURNING %s
`, qualifiedName("tenant", "identity_join_requests"), joinRequestColumnList))).
		WithArgs(
			"join-2",
			"alice",
			"Alice",
			"alice@example.com",
			"12345",
			"new note",
			identity.JoinRequestStatusPending,
			requestedAt,
			nil,
			"",
			"",
			requestedAt.Add(24*time.Hour),
		).
		WillReturnRows(sqlmock.NewRows(strings.Split(joinRequestColumnList, ", ")).AddRow(
			"join-2",
			"alice",
			"Alice",
			"alice@example.com",
			"12345",
			"new note",
			identity.JoinRequestStatusPending,
			requestedAt,
			nil,
			"",
			"",
			requestedAt.Add(24*time.Hour),
		))
	mock.ExpectCommit()

	joinRequest, err := store.SaveJoinRequest(context.Background(), identity.JoinRequest{
		ID:          "join-2",
		Username:    "alice",
		DisplayName: "Alice",
		Email:       "alice@example.com",
		Phone:       "12345",
		Note:        "new note",
		Status:      identity.JoinRequestStatusPending,
		RequestedAt: requestedAt,
		ExpiresAt:   requestedAt.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("save join request: %v", err)
	}
	if joinRequest.ID != "join-2" || joinRequest.Status != identity.JoinRequestStatusPending {
		t.Fatalf("unexpected saved join request: %+v", joinRequest)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
