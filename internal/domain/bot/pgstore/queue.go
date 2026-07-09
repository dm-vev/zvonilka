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

// SaveUpdate inserts or deduplicates one queued update.
func (s *Store) SaveUpdate(ctx context.Context, entry bot.QueueEntry) (bot.QueueEntry, error) {
	if err := s.requireStore(); err != nil {
		return bot.QueueEntry{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.QueueEntry{}, err
	}
	if s.tx != nil {
		return s.saveUpdate(ctx, entry)
	}

	var saved bot.QueueEntry
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveUpdate(ctx, entry)
		return saveErr
	})
	if err != nil {
		return bot.QueueEntry{}, err
	}

	return saved, nil
}

func (s *Store) saveUpdate(ctx context.Context, entry bot.QueueEntry) (bot.QueueEntry, error) {
	now := time.Now().UTC()
	entry, err := bot.NormalizeQueueEntry(entry, now)
	if err != nil {
		return bot.QueueEntry{}, err
	}
	payload, err := encodeUpdate(entry.Payload)
	if err != nil {
		return bot.QueueEntry{}, fmt.Errorf("encode bot update payload: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s AS existing (
	bot_account_id,
	event_id,
	update_type,
	payload,
	attempts,
	next_attempt_at,
	last_error,
	created_at,
	updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (bot_account_id, event_id, update_type) DO UPDATE SET
	payload = existing.payload,
	updated_at = existing.updated_at
RETURNING update_id, bot_account_id, event_id, update_type, payload, attempts, next_attempt_at, last_error, created_at, updated_at
`, s.table("bot_updates"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		entry.BotAccountID,
		entry.EventID,
		entry.UpdateType,
		payload,
		entry.Attempts,
		entry.NextAttemptAt.UTC(),
		entry.LastError,
		entry.CreatedAt.UTC(),
		entry.UpdatedAt.UTC(),
	)

	saved, err := scanUpdate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.QueueEntry{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.QueueEntry{}, mapped
		}

		return bot.QueueEntry{}, fmt.Errorf("save bot update for %s/%s: %w", entry.BotAccountID, entry.EventID, err)
	}

	return saved, nil
}

// PendingUpdates returns queued updates for a bot in update-id order.
func (s *Store) PendingUpdates(
	ctx context.Context,
	botAccountID string,
	offset int64,
	allowed []bot.UpdateType,
	before time.Time,
	limit int,
) ([]bot.QueueEntry, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return nil, bot.ErrInvalidInput
	}
	if limit <= 0 {
		return nil, nil
	}

	args := []any{botAccountID, offset, limit}
	filter := ""
	if len(allowed) > 0 {
		items := make([]string, 0, len(allowed))
		for _, value := range allowed {
			if value == bot.UpdateTypeUnspecified {
				continue
			}
			items = append(items, string(value))
		}
		if len(items) > 0 {
			args = append(args, items)
			filter = fmt.Sprintf(" AND update_type = ANY($%d)", len(args))
		}
	}
	if !before.IsZero() {
		args = append(args, before.UTC())
		filter += fmt.Sprintf(" AND next_attempt_at <= $%d", len(args))
	}

	query := fmt.Sprintf(`
SELECT update_id, bot_account_id, event_id, update_type, payload, attempts, next_attempt_at, last_error, created_at, updated_at
FROM %s
WHERE bot_account_id = $1 AND update_id >= $2%s
ORDER BY update_id ASC
LIMIT $3
`, s.table("bot_updates"), filter)

	rows, err := s.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load pending bot updates for %s: %w", botAccountID, err)
	}
	defer rows.Close()

	result := make([]bot.QueueEntry, 0)
	for rows.Next() {
		entry, err := scanUpdate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bot update: %w", err)
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bot updates for %s: %w", botAccountID, err)
	}

	return result, nil
}

// DeleteUpdatesBefore acknowledges all updates lower than the provided offset.
func (s *Store) DeleteUpdatesBefore(ctx context.Context, botAccountID string, offset int64) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE bot_account_id = $1 AND update_id < $2`, s.table("bot_updates"))
	if _, err := s.conn().ExecContext(ctx, query, botAccountID, offset); err != nil {
		return fmt.Errorf("delete bot updates before %d for %s: %w", offset, botAccountID, err)
	}

	return nil
}

// DeleteUpdate acknowledges one delivered update.
func (s *Store) DeleteUpdate(ctx context.Context, botAccountID string, updateID int64) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" || updateID <= 0 {
		return bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE bot_account_id = $1 AND update_id = $2`, s.table("bot_updates"))
	result, err := s.conn().ExecContext(ctx, query, botAccountID, updateID)
	if err != nil {
		return fmt.Errorf("delete bot update %d for %s: %w", updateID, botAccountID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bot update %d for %s: rows affected: %w", updateID, botAccountID, err)
	}
	if rows == 0 {
		return bot.ErrNotFound
	}

	return nil
}

// DeleteAllUpdates purges all queued updates for a bot.
func (s *Store) DeleteAllUpdates(ctx context.Context, botAccountID string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE bot_account_id = $1`, s.table("bot_updates"))
	if _, err := s.conn().ExecContext(ctx, query, botAccountID); err != nil {
		return fmt.Errorf("delete all bot updates for %s: %w", botAccountID, err)
	}

	return nil
}

// PendingUpdateCount counts the queued bot updates.
func (s *Store) PendingUpdateCount(ctx context.Context, botAccountID string) (int, error) {
	if err := s.requireStore(); err != nil {
		return 0, err
	}
	if err := s.requireContext(ctx); err != nil {
		return 0, err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return 0, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE bot_account_id = $1`, s.table("bot_updates"))
	var count int
	if err := s.conn().QueryRowContext(ctx, query, botAccountID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count bot updates for %s: %w", botAccountID, err)
	}

	return count, nil
}

// RetryUpdate updates one queued update after a webhook failure.
func (s *Store) RetryUpdate(ctx context.Context, params bot.RetryParams) (bot.QueueEntry, error) {
	if err := s.requireStore(); err != nil {
		return bot.QueueEntry{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.QueueEntry{}, err
	}
	params.BotAccountID = strings.TrimSpace(params.BotAccountID)
	if params.BotAccountID == "" || params.UpdateID <= 0 {
		return bot.QueueEntry{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
UPDATE %s
SET attempts = attempts + 1,
	next_attempt_at = $3,
	last_error = $4,
	updated_at = $5
WHERE bot_account_id = $1 AND update_id = $2
RETURNING update_id, bot_account_id, event_id, update_type, payload, attempts, next_attempt_at, last_error, created_at, updated_at
`, s.table("bot_updates"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		params.BotAccountID,
		params.UpdateID,
		params.NextAttemptAt.UTC(),
		params.LastError,
		params.AttemptedAt.UTC(),
	)

	entry, err := scanUpdate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.QueueEntry{}, bot.ErrNotFound
		}
		return bot.QueueEntry{}, fmt.Errorf("retry bot update %d for %s: %w", params.UpdateID, params.BotAccountID, err)
	}

	return entry, nil
}
