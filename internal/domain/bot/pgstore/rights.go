package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (s *Store) SaveRights(ctx context.Context, state bot.AdminRightsState) (bot.AdminRightsState, error) {
	if err := s.requireStore(); err != nil {
		return bot.AdminRightsState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.AdminRightsState{}, err
	}
	if s.tx != nil {
		return s.saveRights(ctx, state)
	}

	var saved bot.AdminRightsState
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveRights(ctx, state)
		return saveErr
	})
	if err != nil {
		return bot.AdminRightsState{}, err
	}

	return saved, nil
}

func (s *Store) saveRights(ctx context.Context, state bot.AdminRightsState) (bot.AdminRightsState, error) {
	state.BotAccountID = strings.TrimSpace(state.BotAccountID)
	if state.BotAccountID == "" {
		return bot.AdminRightsState{}, bot.ErrInvalidInput
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = time.Now().UTC()
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = state.CreatedAt
	}

	raw, err := json.Marshal(state.Rights)
	if err != nil {
		return bot.AdminRightsState{}, fmt.Errorf("encode bot admin rights: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	bot_account_id, for_channels, rights, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (bot_account_id, for_channels) DO UPDATE SET
	rights = EXCLUDED.rights,
	updated_at = EXCLUDED.updated_at
RETURNING bot_account_id, for_channels, rights, created_at, updated_at
`, s.table("bot_default_admin_rights"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		state.BotAccountID,
		state.ForChannels,
		raw,
		state.CreatedAt.UTC(),
		state.UpdatedAt.UTC(),
	)

	saved, err := scanRights(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.AdminRightsState{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.AdminRightsState{}, mapped
		}

		return bot.AdminRightsState{}, fmt.Errorf("save bot admin rights for %s: %w", state.BotAccountID, err)
	}

	return saved, nil
}

func (s *Store) RightsByScope(ctx context.Context, botAccountID string, forChannels bool) (bot.AdminRightsState, error) {
	if err := s.requireStore(); err != nil {
		return bot.AdminRightsState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.AdminRightsState{}, err
	}

	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return bot.AdminRightsState{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT bot_account_id, for_channels, rights, created_at, updated_at
FROM %s
WHERE bot_account_id = $1 AND for_channels = $2
`, s.table("bot_default_admin_rights"))

	row := s.conn().QueryRowContext(ctx, query, botAccountID, forChannels)
	state, err := scanRights(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.AdminRightsState{}, bot.ErrNotFound
		}

		return bot.AdminRightsState{}, fmt.Errorf("load bot admin rights for %s: %w", botAccountID, err)
	}

	return state, nil
}

func (s *Store) DeleteRights(ctx context.Context, botAccountID string, forChannels bool) error {
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

	query := fmt.Sprintf(`
DELETE FROM %s
WHERE bot_account_id = $1 AND for_channels = $2
`, s.table("bot_default_admin_rights"))
	result, err := s.conn().ExecContext(ctx, query, botAccountID, forChannels)
	if err != nil {
		return fmt.Errorf("delete bot admin rights for %s: %w", botAccountID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bot admin rights for %s: rows affected: %w", botAccountID, err)
	}
	if rows == 0 {
		return bot.ErrNotFound
	}

	return nil
}

func scanRights(row rowScanner) (bot.AdminRightsState, error) {
	var (
		state bot.AdminRightsState
		raw   []byte
	)
	if err := row.Scan(
		&state.BotAccountID,
		&state.ForChannels,
		&raw,
		&state.CreatedAt,
		&state.UpdatedAt,
	); err != nil {
		return bot.AdminRightsState{}, err
	}
	if err := json.Unmarshal(raw, &state.Rights); err != nil {
		return bot.AdminRightsState{}, err
	}

	return state, nil
}
