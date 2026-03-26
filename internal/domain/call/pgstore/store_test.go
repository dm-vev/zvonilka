package pgstore

import (
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	store, err := New(db, "call")
	require.NoError(t, err)

	return store, mock, db
}

func TestMapConstraintError(t *testing.T) {
	t.Parallel()

	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23505"}), call.ErrConflict))
	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23503"}), call.ErrNotFound))
	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23514"}), call.ErrInvalidInput))
	require.Nil(t, mapConstraintError(&pgconn.PgError{Code: "99999"}))
}
