package identity_test

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
)

const pgstoreJoinRequestColumns = "id, username, display_name, email, phone, note, status, requested_at, reviewed_at, reviewed_by, decision_reason, expires_at"

func newPGStore(t *testing.T) (*pgstore.Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}

	store, err := pgstore.New(db, "tenant")
	if err != nil {
		t.Fatalf("new pgstore: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return store, mock, db
}

func qualifiedName(schema string, name string) string {
	return fmt.Sprintf(`"%s"."%s"`, schema, name)
}

func TestSaveJoinRequestExpiresStalePendingRequestByCurrentTime(t *testing.T) {
	t.Parallel()

	store, mock, _ := newPGStore(t)

	now := time.Date(2026, time.March, 23, 18, 0, 0, 0, time.UTC)
	requestedAt := now.Add(-4 * time.Hour)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE username = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("backdated-user").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`SELECT id FROM %s WHERE email = $1 LIMIT 1`, qualifiedName("tenant", "identity_accounts")))).
		WithArgs("backdated@example.com").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(`
UPDATE %s
SET status = $2,
    reviewed_at = CURRENT_TIMESTAMP,
    reviewed_by = '',
    decision_reason = 'join request expired'
WHERE status = $1
  AND expires_at <= CURRENT_TIMESTAMP
  AND (username = $3 OR email = $4)
`, qualifiedName("tenant", "identity_join_requests")))).
		WithArgs(
			identity.JoinRequestStatusPending,
			identity.JoinRequestStatusExpired,
			"backdated-user",
			"backdated@example.com",
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
`, qualifiedName("tenant", "identity_join_requests"), pgstoreJoinRequestColumns))).
		WithArgs(
			"join-2",
			"backdated-user",
			"Backdated User",
			"backdated@example.com",
			"",
			"",
			identity.JoinRequestStatusPending,
			requestedAt,
			nil,
			"",
			"",
			now.Add(24*time.Hour),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"username",
			"display_name",
			"email",
			"phone",
			"note",
			"status",
			"requested_at",
			"reviewed_at",
			"reviewed_by",
			"decision_reason",
			"expires_at",
		}).AddRow(
			"join-2",
			"backdated-user",
			"Backdated User",
			"backdated@example.com",
			"",
			"",
			identity.JoinRequestStatusPending,
			requestedAt,
			nil,
			"",
			"",
			now.Add(24*time.Hour),
		))
	mock.ExpectCommit()

	joinRequest, err := store.SaveJoinRequest(context.Background(), identity.JoinRequest{
		ID:          "join-2",
		Username:    "backdated-user",
		DisplayName: "Backdated User",
		Email:       "backdated@example.com",
		Status:      identity.JoinRequestStatusPending,
		RequestedAt: requestedAt,
		ExpiresAt:   now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("save join request: %v", err)
	}
	if joinRequest.ID != "join-2" {
		t.Fatalf("unexpected join request: %+v", joinRequest)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
