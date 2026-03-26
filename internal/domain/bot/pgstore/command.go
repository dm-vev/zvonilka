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

func (s *Store) SaveCommands(ctx context.Context, set bot.CommandSet) (bot.CommandSet, error) {
	if err := s.requireStore(); err != nil {
		return bot.CommandSet{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.CommandSet{}, err
	}
	if s.tx != nil {
		return s.saveCommands(ctx, set)
	}

	var saved bot.CommandSet
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveCommands(ctx, set)
		return saveErr
	})
	if err != nil {
		return bot.CommandSet{}, err
	}

	return saved, nil
}

func (s *Store) saveCommands(ctx context.Context, set bot.CommandSet) (bot.CommandSet, error) {
	now := time.Now().UTC()
	set, err := bot.NormalizeCommandSet(set, now)
	if err != nil {
		return bot.CommandSet{}, err
	}
	raw, err := json.Marshal(set.Commands)
	if err != nil {
		return bot.CommandSet{}, fmt.Errorf("encode bot commands: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	bot_account_id, scope_type, scope_chat_id, scope_user_id, language_code, commands, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (bot_account_id, scope_type, scope_chat_id, scope_user_id, language_code) DO UPDATE SET
	commands = EXCLUDED.commands,
	updated_at = EXCLUDED.updated_at
RETURNING bot_account_id, scope_type, scope_chat_id, scope_user_id, language_code, commands, created_at, updated_at
`, s.table("bot_commands"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		set.BotAccountID,
		set.Scope.Type,
		set.Scope.ChatID,
		set.Scope.UserID,
		set.LanguageCode,
		raw,
		set.CreatedAt.UTC(),
		set.UpdatedAt.UTC(),
	)

	saved, err := scanCommands(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.CommandSet{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.CommandSet{}, mapped
		}
		return bot.CommandSet{}, fmt.Errorf("save bot commands for %s: %w", set.BotAccountID, err)
	}

	return saved, nil
}

func (s *Store) CommandsByScope(
	ctx context.Context,
	botAccountID string,
	scope bot.CommandScope,
	languageCode string,
) (bot.CommandSet, error) {
	if err := s.requireStore(); err != nil {
		return bot.CommandSet{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.CommandSet{}, err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	scope, languageCode, err := bot.NormalizeCommandLookup(scope, languageCode)
	if err != nil {
		return bot.CommandSet{}, err
	}
	if botAccountID == "" {
		return bot.CommandSet{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT bot_account_id, scope_type, scope_chat_id, scope_user_id, language_code, commands, created_at, updated_at
FROM %s
WHERE bot_account_id = $1 AND scope_type = $2 AND scope_chat_id = $3 AND scope_user_id = $4 AND language_code = $5
`, s.table("bot_commands"))

	row := s.conn().QueryRowContext(ctx, query, botAccountID, scope.Type, scope.ChatID, scope.UserID, languageCode)
	set, err := scanCommands(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.CommandSet{}, bot.ErrNotFound
		}
		return bot.CommandSet{}, fmt.Errorf("load bot commands for %s: %w", botAccountID, err)
	}

	return set, nil
}

func (s *Store) DeleteCommands(
	ctx context.Context,
	botAccountID string,
	scope bot.CommandScope,
	languageCode string,
) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	scope, languageCode, err := bot.NormalizeCommandLookup(scope, languageCode)
	if err != nil {
		return err
	}
	if botAccountID == "" {
		return bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
DELETE FROM %s
WHERE bot_account_id = $1 AND scope_type = $2 AND scope_chat_id = $3 AND scope_user_id = $4 AND language_code = $5
`, s.table("bot_commands"))
	result, err := s.conn().ExecContext(ctx, query, botAccountID, scope.Type, scope.ChatID, scope.UserID, languageCode)
	if err != nil {
		return fmt.Errorf("delete bot commands for %s: %w", botAccountID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bot commands for %s: rows affected: %w", botAccountID, err)
	}
	if rows == 0 {
		return bot.ErrNotFound
	}

	return nil
}

func scanCommands(row rowScanner) (bot.CommandSet, error) {
	var (
		set bot.CommandSet
		raw []byte
	)

	if err := row.Scan(
		&set.BotAccountID,
		&set.Scope.Type,
		&set.Scope.ChatID,
		&set.Scope.UserID,
		&set.LanguageCode,
		&raw,
		&set.CreatedAt,
		&set.UpdatedAt,
	); err != nil {
		return bot.CommandSet{}, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &set.Commands); err != nil {
			return bot.CommandSet{}, err
		}
	}

	return set, nil
}
