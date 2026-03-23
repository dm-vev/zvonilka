package pgstore

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

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
