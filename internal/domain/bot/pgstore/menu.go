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

func (s *Store) SaveMenu(ctx context.Context, state bot.MenuState) (bot.MenuState, error) {
	if err := s.requireStore(); err != nil {
		return bot.MenuState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.MenuState{}, err
	}
	if s.tx != nil {
		return s.saveMenu(ctx, state)
	}

	var saved bot.MenuState
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveMenu(ctx, state)
		return saveErr
	})
	if err != nil {
		return bot.MenuState{}, err
	}

	return saved, nil
}

func (s *Store) saveMenu(ctx context.Context, state bot.MenuState) (bot.MenuState, error) {
	now := time.Now().UTC()
	state, err := bot.NormalizeMenuState(state, now)
	if err != nil {
		return bot.MenuState{}, err
	}
	raw, err := json.Marshal(state.Button)
	if err != nil {
		return bot.MenuState{}, fmt.Errorf("encode bot menu button: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	bot_account_id, chat_id, button, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (bot_account_id, chat_id) DO UPDATE SET
	button = EXCLUDED.button,
	updated_at = EXCLUDED.updated_at
RETURNING bot_account_id, chat_id, button, created_at, updated_at
`, s.table("bot_menu_buttons"))

	row := s.conn().QueryRowContext(ctx, query, state.BotAccountID, state.ChatID, raw, state.CreatedAt.UTC(), state.UpdatedAt.UTC())
	saved, err := scanMenu(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.MenuState{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.MenuState{}, mapped
		}
		return bot.MenuState{}, fmt.Errorf("save bot menu button for %s: %w", state.BotAccountID, err)
	}

	return saved, nil
}

func (s *Store) MenuByChat(ctx context.Context, botAccountID string, chatID string) (bot.MenuState, error) {
	if err := s.requireStore(); err != nil {
		return bot.MenuState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.MenuState{}, err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	chatID = strings.TrimSpace(chatID)
	if botAccountID == "" {
		return bot.MenuState{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT bot_account_id, chat_id, button, created_at, updated_at
FROM %s
WHERE bot_account_id = $1 AND chat_id = $2
`, s.table("bot_menu_buttons"))
	row := s.conn().QueryRowContext(ctx, query, botAccountID, chatID)
	state, err := scanMenu(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.MenuState{}, bot.ErrNotFound
		}
		return bot.MenuState{}, fmt.Errorf("load bot menu button for %s: %w", botAccountID, err)
	}

	return state, nil
}

func scanMenu(row rowScanner) (bot.MenuState, error) {
	var (
		state bot.MenuState
		raw   []byte
	)

	if err := row.Scan(&state.BotAccountID, &state.ChatID, &raw, &state.CreatedAt, &state.UpdatedAt); err != nil {
		return bot.MenuState{}, err
	}
	if err := json.Unmarshal(raw, &state.Button); err != nil {
		return bot.MenuState{}, err
	}

	return state, nil
}
