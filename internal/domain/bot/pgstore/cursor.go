package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

// SaveCursor stores the worker sequence watermark.
func (s *Store) SaveCursor(ctx context.Context, cursor bot.Cursor) (bot.Cursor, error) {
	if err := s.requireStore(); err != nil {
		return bot.Cursor{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.Cursor{}, err
	}
	if s.tx != nil {
		return s.saveCursor(ctx, cursor)
	}

	var saved bot.Cursor
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveCursor(ctx, cursor)
		return saveErr
	})
	if err != nil {
		return bot.Cursor{}, err
	}

	return saved, nil
}

func (s *Store) saveCursor(ctx context.Context, cursor bot.Cursor) (bot.Cursor, error) {
	now := time.Now().UTC()
	cursor, err := bot.NormalizeCursor(cursor, now)
	if err != nil {
		return bot.Cursor{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s AS existing (name, last_sequence, updated_at)
VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE SET
	last_sequence = GREATEST(existing.last_sequence, EXCLUDED.last_sequence),
	updated_at = CASE
		WHEN EXCLUDED.last_sequence > existing.last_sequence THEN EXCLUDED.updated_at
		ELSE existing.updated_at
	END
RETURNING name, last_sequence, updated_at
`, s.table("bot_worker_cursors"))

	row := s.conn().QueryRowContext(ctx, query, cursor.Name, cursor.LastSequence, cursor.UpdatedAt.UTC())
	saved, err := scanCursor(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.Cursor{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.Cursor{}, mapped
		}

		return bot.Cursor{}, fmt.Errorf("save bot cursor %s: %w", cursor.Name, err)
	}

	return saved, nil
}

// CursorByName resolves the worker cursor by logical name.
func (s *Store) CursorByName(ctx context.Context, name string) (bot.Cursor, error) {
	if err := s.requireStore(); err != nil {
		return bot.Cursor{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.Cursor{}, err
	}
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return bot.Cursor{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT name, last_sequence, updated_at FROM %s WHERE name = $1`, s.table("bot_worker_cursors"))
	row := s.conn().QueryRowContext(ctx, query, name)
	cursor, err := scanCursor(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.Cursor{}, bot.ErrNotFound
		}
		return bot.Cursor{}, fmt.Errorf("load bot cursor %s: %w", name, err)
	}

	return cursor, nil
}
