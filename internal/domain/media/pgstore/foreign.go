package pgstore

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/media"
)

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func isConstraintError(err error, code string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == code {
		return true
	}

	return false
}

func mapConstraintError(err error, fallback error) error {
	if err == nil {
		return nil
	}

	switch {
	case isConstraintError(err, "23505"):
		return media.ErrConflict
	case isConstraintError(err, "23514"):
		return media.ErrInvalidInput
	case isConstraintError(err, "23503"):
		if fallback != nil {
			return fallback
		}
		return media.ErrNotFound
	default:
		return nil
	}
}
