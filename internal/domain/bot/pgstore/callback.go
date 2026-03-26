package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

// SaveCallback inserts or updates one callback query state.
func (s *Store) SaveCallback(ctx context.Context, callback bot.Callback) (bot.Callback, error) {
	if err := s.requireStore(); err != nil {
		return bot.Callback{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.Callback{}, err
	}
	if s.tx != nil {
		return s.saveCallback(ctx, callback)
	}

	var saved bot.Callback
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveCallback(ctx, callback)
		return saveErr
	})
	if err != nil {
		return bot.Callback{}, err
	}

	return saved, nil
}

func (s *Store) saveCallback(ctx context.Context, callback bot.Callback) (bot.Callback, error) {
	now := time.Now().UTC()
	callback, err := bot.NormalizeCallback(callback, now)
	if err != nil {
		return bot.Callback{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, bot_account_id, from_account_id, conversation_id, message_id, message_thread_id, chat_instance, data,
	answered_text, answered_url, show_alert, cache_time_seconds, created_at, updated_at, answered_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
ON CONFLICT (id) DO UPDATE SET
	answered_text = EXCLUDED.answered_text,
	answered_url = EXCLUDED.answered_url,
	show_alert = EXCLUDED.show_alert,
	cache_time_seconds = EXCLUDED.cache_time_seconds,
	updated_at = EXCLUDED.updated_at,
	answered_at = EXCLUDED.answered_at
RETURNING id, bot_account_id, from_account_id, conversation_id, message_id, message_thread_id, chat_instance, data,
	answered_text, answered_url, show_alert, cache_time_seconds, created_at, updated_at, answered_at
`, s.table("bot_callbacks"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		callback.ID,
		callback.BotAccountID,
		callback.FromAccountID,
		callback.ConversationID,
		callback.MessageID,
		callback.MessageThreadID,
		callback.ChatInstance,
		callback.Data,
		callback.AnsweredText,
		callback.AnsweredURL,
		callback.ShowAlert,
		callback.CacheTime,
		callback.CreatedAt.UTC(),
		callback.UpdatedAt.UTC(),
		encodeTime(callback.AnsweredAt),
	)

	saved, err := scanCallback(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.Callback{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.Callback{}, mapped
		}
		return bot.Callback{}, fmt.Errorf("save callback query %s: %w", callback.ID, err)
	}

	return saved, nil
}

// CallbackByID loads one callback query state.
func (s *Store) CallbackByID(ctx context.Context, callbackID string) (bot.Callback, error) {
	if err := s.requireStore(); err != nil {
		return bot.Callback{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.Callback{}, err
	}
	if callbackID == "" {
		return bot.Callback{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT id, bot_account_id, from_account_id, conversation_id, message_id, message_thread_id, chat_instance, data,
	answered_text, answered_url, show_alert, cache_time_seconds, created_at, updated_at, answered_at
FROM %s
WHERE id = $1
`, s.table("bot_callbacks"))

	row := s.conn().QueryRowContext(ctx, query, callbackID)
	callback, err := scanCallback(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.Callback{}, bot.ErrNotFound
		}
		return bot.Callback{}, fmt.Errorf("load callback query %s: %w", callbackID, err)
	}

	return callback, nil
}

// AnswerCallback updates the answer state for one callback query.
func (s *Store) AnswerCallback(ctx context.Context, callback bot.Callback) (bot.Callback, error) {
	return s.SaveCallback(ctx, callback)
}
