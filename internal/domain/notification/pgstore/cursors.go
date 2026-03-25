package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

// SaveWorkerCursor stores the worker's last processed sequence.
func (s *Store) SaveWorkerCursor(ctx context.Context, cursor notification.WorkerCursor) (notification.WorkerCursor, error) {
	if err := s.requireStore(); err != nil {
		return notification.WorkerCursor{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.WorkerCursor{}, err
	}
	if s.tx != nil {
		return s.saveWorkerCursor(ctx, cursor)
	}

	var saved notification.WorkerCursor
	err := s.WithinTx(ctx, func(tx notification.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveWorkerCursor(ctx, cursor)
		return saveErr
	})
	if err != nil {
		return notification.WorkerCursor{}, err
	}

	return saved, nil
}

func (s *Store) saveWorkerCursor(ctx context.Context, cursor notification.WorkerCursor) (notification.WorkerCursor, error) {
	now := time.Now().UTC()
	cursor, err := notification.NormalizeWorkerCursor(cursor, now)
	if err != nil {
		return notification.WorkerCursor{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	name,
	last_sequence,
	updated_at
) VALUES (
	$1, $2, $3
)
ON CONFLICT (name) DO UPDATE SET
	last_sequence = EXCLUDED.last_sequence,
	updated_at = EXCLUDED.updated_at
RETURNING name, last_sequence, updated_at
`, s.table("notification_worker_cursors"))

	row := s.conn().QueryRowContext(ctx, query, cursor.Name, cursor.LastSequence, cursor.UpdatedAt.UTC())
	saved, err := scanCursor(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.WorkerCursor{}, notification.ErrNotFound
		}
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return notification.WorkerCursor{}, mappedErr
		}

		return notification.WorkerCursor{}, fmt.Errorf("save worker cursor %s: %w", cursor.Name, err)
	}

	return saved, nil
}

// WorkerCursorByName resolves the worker cursor by logical worker name.
func (s *Store) WorkerCursorByName(ctx context.Context, name string) (notification.WorkerCursor, error) {
	if err := s.requireStore(); err != nil {
		return notification.WorkerCursor{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.WorkerCursor{}, err
	}
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return notification.WorkerCursor{}, notification.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT name, last_sequence, updated_at
FROM %s
WHERE name = $1
`, s.table("notification_worker_cursors"))

	row := s.conn().QueryRowContext(ctx, query, name)
	cursor, err := scanCursor(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.WorkerCursor{}, notification.ErrNotFound
		}
		return notification.WorkerCursor{}, fmt.Errorf("load worker cursor %s: %w", name, err)
	}

	return cursor, nil
}
