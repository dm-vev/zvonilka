package pgstore

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

func TestSaveSessionCredentialUpsertsBySessionAndKind(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)
	now := time.Date(2026, time.March, 26, 14, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(`
INSERT INTO %s (
	session_id, account_id, device_id, kind, token_hash, expires_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (session_id, kind) DO UPDATE SET
	account_id = EXCLUDED.account_id,
	device_id = EXCLUDED.device_id,
	token_hash = EXCLUDED.token_hash,
	expires_at = EXCLUDED.expires_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, qualifiedName("tenant", "identity_session_credentials"), credentialColumnList))).
		WithArgs(
			"sess-1",
			"acc-1",
			"dev-1",
			identity.SessionCredentialKindAccess,
			"hash-1",
			now.Add(30*time.Minute),
			now,
			now,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"session_id",
			"account_id",
			"device_id",
			"kind",
			"token_hash",
			"expires_at",
			"created_at",
			"updated_at",
		}).AddRow(
			"sess-1",
			"acc-1",
			"dev-1",
			identity.SessionCredentialKindAccess,
			"hash-1",
			now.Add(30*time.Minute),
			now,
			now,
		))

	credential, err := store.SaveSessionCredential(context.Background(), identity.SessionCredential{
		SessionID: "sess-1",
		AccountID: "acc-1",
		DeviceID:  "dev-1",
		Kind:      identity.SessionCredentialKindAccess,
		TokenHash: "hash-1",
		ExpiresAt: now.Add(30 * time.Minute),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("save session credential: %v", err)
	}
	if credential.SessionID != "sess-1" || credential.Kind != identity.SessionCredentialKindAccess {
		t.Fatalf("unexpected credential: %+v", credential)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSessionCredentialByTokenHashReturnsNotFound(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectQuery(regexp.QuoteMeta(fmt.Sprintf(
		`SELECT %s FROM %s WHERE token_hash = $1 AND kind = $2`,
		credentialColumnList,
		qualifiedName("tenant", "identity_session_credentials"),
	))).
		WithArgs("hash-1", identity.SessionCredentialKindRefresh).
		WillReturnRows(sqlmock.NewRows([]string{
			"session_id",
			"account_id",
			"device_id",
			"kind",
			"token_hash",
			"expires_at",
			"created_at",
			"updated_at",
		}))

	_, err := store.SessionCredentialByTokenHash(
		context.Background(),
		"hash-1",
		identity.SessionCredentialKindRefresh,
	)
	if !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestDeleteSessionCredentialsBySessionIDRemovesRows(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		`DELETE FROM %s WHERE session_id = $1`,
		qualifiedName("tenant", "identity_session_credentials"),
	))).
		WithArgs("sess-1").
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := store.DeleteSessionCredentialsBySessionID(context.Background(), "sess-1"); err != nil {
		t.Fatalf("delete session credentials: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
