package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

// SaveWorkerCursor stores the worker's last processed call-event sequence.
func (s *Store) SaveWorkerCursor(ctx context.Context, cursor call.WorkerCursor) (call.WorkerCursor, error) {
	if err := s.requireStore(); err != nil {
		return call.WorkerCursor{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.WorkerCursor{}, err
	}
	if s.tx != nil {
		return s.saveWorkerCursor(ctx, cursor)
	}

	var saved call.WorkerCursor
	err := s.WithinTx(ctx, func(tx call.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveWorkerCursor(ctx, cursor)
		return saveErr
	})
	if err != nil {
		return call.WorkerCursor{}, err
	}

	return saved, nil
}

func (s *Store) saveWorkerCursor(ctx context.Context, cursor call.WorkerCursor) (call.WorkerCursor, error) {
	now := time.Now().UTC()
	cursor, err := call.NormalizeWorkerCursor(cursor, now)
	if err != nil {
		return call.WorkerCursor{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s AS existing (
	name,
	last_sequence,
	updated_at
) VALUES (
	$1, $2, $3
)
ON CONFLICT (name) DO UPDATE SET
	last_sequence = GREATEST(existing.last_sequence, EXCLUDED.last_sequence),
	updated_at = CASE
		WHEN EXCLUDED.last_sequence > existing.last_sequence THEN EXCLUDED.updated_at
		ELSE existing.updated_at
	END
RETURNING name, last_sequence, updated_at
`, s.table("call_worker_cursors"))

	row := s.conn().QueryRowContext(ctx, query, cursor.Name, cursor.LastSequence, cursor.UpdatedAt.UTC())
	saved, err := scanWorkerCursor(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return call.WorkerCursor{}, call.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return call.WorkerCursor{}, mapped
		}

		return call.WorkerCursor{}, fmt.Errorf("save worker cursor %s: %w", cursor.Name, err)
	}

	return saved, nil
}

// WorkerCursorByName resolves the worker cursor by logical worker name.
func (s *Store) WorkerCursorByName(ctx context.Context, name string) (call.WorkerCursor, error) {
	if err := s.requireStore(); err != nil {
		return call.WorkerCursor{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.WorkerCursor{}, err
	}
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return call.WorkerCursor{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT name, last_sequence, updated_at
FROM %s
WHERE name = $1
`, s.table("call_worker_cursors"))

	row := s.conn().QueryRowContext(ctx, query, name)
	cursor, err := scanWorkerCursor(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return call.WorkerCursor{}, call.ErrNotFound
		}
		return call.WorkerCursor{}, fmt.Errorf("load worker cursor %s: %w", name, err)
	}

	return cursor, nil
}
