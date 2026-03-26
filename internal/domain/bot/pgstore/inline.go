package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

// SaveInlineQuery inserts or updates one inline query state.
func (s *Store) SaveInlineQuery(ctx context.Context, query bot.InlineQueryState) (bot.InlineQueryState, error) {
	if err := s.requireStore(); err != nil {
		return bot.InlineQueryState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.InlineQueryState{}, err
	}
	if s.tx != nil {
		return s.saveInlineQuery(ctx, query)
	}

	var saved bot.InlineQueryState
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveInlineQuery(ctx, query)
		return saveErr
	})
	if err != nil {
		return bot.InlineQueryState{}, err
	}

	return saved, nil
}

func (s *Store) saveInlineQuery(ctx context.Context, query bot.InlineQueryState) (bot.InlineQueryState, error) {
	now := time.Now().UTC()
	query, err := bot.NormalizeInlineQuery(query, now)
	if err != nil {
		return bot.InlineQueryState{}, err
	}
	rawResults, err := encodeInlineResults(query.Results)
	if err != nil {
		return bot.InlineQueryState{}, err
	}

	sqlQuery := fmt.Sprintf(`
INSERT INTO %s (
	id, bot_account_id, from_account_id, query_text, query_offset, chat_type, answered,
	results_json, cache_time_seconds, is_personal, next_offset, switch_pm_text, switch_pm_param,
	created_at, updated_at, answered_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12,$13,$14,$15,$16)
ON CONFLICT (id) DO UPDATE SET
	answered = EXCLUDED.answered,
	results_json = EXCLUDED.results_json,
	cache_time_seconds = EXCLUDED.cache_time_seconds,
	is_personal = EXCLUDED.is_personal,
	next_offset = EXCLUDED.next_offset,
	switch_pm_text = EXCLUDED.switch_pm_text,
	switch_pm_param = EXCLUDED.switch_pm_param,
	updated_at = EXCLUDED.updated_at,
	answered_at = EXCLUDED.answered_at
RETURNING id, bot_account_id, from_account_id, query_text, query_offset, chat_type, answered,
	results_json, cache_time_seconds, is_personal, next_offset, switch_pm_text, switch_pm_param,
	created_at, updated_at, answered_at
`, s.table("bot_inline_queries"))

	row := s.conn().QueryRowContext(
		ctx,
		sqlQuery,
		query.ID,
		query.BotAccountID,
		query.FromAccountID,
		query.Query,
		query.Offset,
		query.ChatType,
		query.Answered,
		rawResults,
		query.CacheTime,
		query.IsPersonal,
		query.NextOffset,
		query.SwitchPMText,
		query.SwitchPMParam,
		query.CreatedAt.UTC(),
		query.UpdatedAt.UTC(),
		encodeTime(query.AnsweredAt),
	)

	saved, err := scanInlineQuery(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.InlineQueryState{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.InlineQueryState{}, mapped
		}
		return bot.InlineQueryState{}, fmt.Errorf("save inline query %s: %w", query.ID, err)
	}

	return saved, nil
}

// InlineQueryByID loads one inline query state.
func (s *Store) InlineQueryByID(ctx context.Context, queryID string) (bot.InlineQueryState, error) {
	if err := s.requireStore(); err != nil {
		return bot.InlineQueryState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.InlineQueryState{}, err
	}
	if queryID == "" {
		return bot.InlineQueryState{}, bot.ErrInvalidInput
	}

	sqlQuery := fmt.Sprintf(`
SELECT id, bot_account_id, from_account_id, query_text, query_offset, chat_type, answered,
	results_json, cache_time_seconds, is_personal, next_offset, switch_pm_text, switch_pm_param,
	created_at, updated_at, answered_at
FROM %s
WHERE id = $1
`, s.table("bot_inline_queries"))

	row := s.conn().QueryRowContext(ctx, sqlQuery, queryID)
	query, err := scanInlineQuery(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.InlineQueryState{}, bot.ErrNotFound
		}
		return bot.InlineQueryState{}, fmt.Errorf("load inline query %s: %w", queryID, err)
	}

	return query, nil
}

// AnswerInlineQuery updates one answered inline query state.
func (s *Store) AnswerInlineQuery(ctx context.Context, query bot.InlineQueryState) (bot.InlineQueryState, error) {
	return s.SaveInlineQuery(ctx, query)
}
