package pgstore

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

func TestDeleteLoginChallengeRemovesRow(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		`DELETE FROM %s WHERE id = $1`,
		qualifiedName("tenant", "identity_login_challenges"),
	))).
		WithArgs("chal-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.DeleteLoginChallenge(context.Background(), "chal-1"); err != nil {
		t.Fatalf("delete login challenge: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestDeleteLoginChallengeReturnsNotFound(t *testing.T) {
	t.Parallel()

	store, mock, _ := newMockStore(t)

	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		`DELETE FROM %s WHERE id = $1`,
		qualifiedName("tenant", "identity_login_challenges"),
	))).
		WithArgs("chal-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := store.DeleteLoginChallenge(context.Background(), "chal-1"); !errors.Is(err, identity.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestDeleteLoginChallengeRejectsNilContext(t *testing.T) {
	t.Parallel()

	store, _, _ := newMockStore(t)

	var ctx context.Context
	if err := store.DeleteLoginChallenge(ctx, "chal-1"); !errors.Is(err, identity.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
