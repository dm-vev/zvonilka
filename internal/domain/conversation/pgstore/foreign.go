package pgstore

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

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
		return conversation.ErrConflict
	case isConstraintError(err, "23514"):
		return conversation.ErrInvalidInput
	case isConstraintError(err, "23503"):
		if fallback != nil {
			return fallback
		}
		return conversation.ErrNotFound
	default:
		return nil
	}
}
