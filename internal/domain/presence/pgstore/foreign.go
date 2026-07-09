package pgstore

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/presence"
)

func isConstraintError(err error, code string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == code
}

func mapConstraintError(err error, fallback error) error {
	switch {
	case isConstraintError(err, "23505"):
		return presence.ErrInvalidInput
	case isConstraintError(err, "23503"):
		return fallback
	case isConstraintError(err, "23514"):
		return presence.ErrInvalidInput
	default:
		return nil
	}
}
