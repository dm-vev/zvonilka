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

func (s *Store) SaveProfile(ctx context.Context, value bot.ProfileValue) (bot.ProfileValue, error) {
	if err := s.requireStore(); err != nil {
		return bot.ProfileValue{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.ProfileValue{}, err
	}
	if s.tx != nil {
		return s.saveProfile(ctx, value)
	}

	var saved bot.ProfileValue
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveProfile(ctx, value)
		return saveErr
	})
	if err != nil {
		return bot.ProfileValue{}, err
	}

	return saved, nil
}

func (s *Store) saveProfile(ctx context.Context, value bot.ProfileValue) (bot.ProfileValue, error) {
	now := time.Now().UTC()
	kind, languageCode, normalizedValue, err := bot.NormalizeProfileInput(value.Kind, value.LanguageCode, value.Value)
	if err != nil {
		return bot.ProfileValue{}, err
	}
	value.BotAccountID = strings.TrimSpace(value.BotAccountID)
	value.Kind = kind
	value.LanguageCode = languageCode
	value.Value = normalizedValue
	if value.BotAccountID == "" || value.Value == "" {
		return bot.ProfileValue{}, bot.ErrInvalidInput
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	if value.UpdatedAt.IsZero() {
		value.UpdatedAt = value.CreatedAt
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	bot_account_id, profile_kind, language_code, value, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (bot_account_id, profile_kind, language_code) DO UPDATE SET
	value = EXCLUDED.value,
	updated_at = EXCLUDED.updated_at
RETURNING bot_account_id, profile_kind, language_code, value, created_at, updated_at
`, s.table("bot_profiles"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		value.BotAccountID,
		value.Kind,
		value.LanguageCode,
		value.Value,
		value.CreatedAt.UTC(),
		value.UpdatedAt.UTC(),
	)

	saved, err := scanProfile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.ProfileValue{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.ProfileValue{}, mapped
		}

		return bot.ProfileValue{}, fmt.Errorf("save bot profile for %s: %w", value.BotAccountID, err)
	}

	return saved, nil
}

func (s *Store) ProfileByLanguage(
	ctx context.Context,
	botAccountID string,
	kind bot.ProfileKind,
	languageCode string,
) (bot.ProfileValue, error) {
	if err := s.requireStore(); err != nil {
		return bot.ProfileValue{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.ProfileValue{}, err
	}

	normalizedKind, normalizedLanguageCode, _, err := bot.NormalizeProfileInput(kind, languageCode, "value")
	if err != nil {
		return bot.ProfileValue{}, err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return bot.ProfileValue{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT bot_account_id, profile_kind, language_code, value, created_at, updated_at
FROM %s
WHERE bot_account_id = $1 AND profile_kind = $2 AND language_code = $3
`, s.table("bot_profiles"))

	row := s.conn().QueryRowContext(ctx, query, botAccountID, normalizedKind, normalizedLanguageCode)
	value, err := scanProfile(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.ProfileValue{}, bot.ErrNotFound
		}

		return bot.ProfileValue{}, fmt.Errorf("load bot profile for %s: %w", botAccountID, err)
	}

	return value, nil
}

func (s *Store) DeleteProfile(ctx context.Context, botAccountID string, kind bot.ProfileKind, languageCode string) error {
	if err := s.requireStore(); err != nil {
		return err
	}
	if err := s.requireContext(ctx); err != nil {
		return err
	}

	normalizedKind, normalizedLanguageCode, _, err := bot.NormalizeProfileInput(kind, languageCode, "value")
	if err != nil {
		return err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
DELETE FROM %s
WHERE bot_account_id = $1 AND profile_kind = $2 AND language_code = $3
`, s.table("bot_profiles"))
	result, err := s.conn().ExecContext(ctx, query, botAccountID, normalizedKind, normalizedLanguageCode)
	if err != nil {
		return fmt.Errorf("delete bot profile for %s: %w", botAccountID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bot profile for %s: rows affected: %w", botAccountID, err)
	}
	if rows == 0 {
		return bot.ErrNotFound
	}

	return nil
}

func scanProfile(row rowScanner) (bot.ProfileValue, error) {
	var value bot.ProfileValue
	if err := row.Scan(
		&value.BotAccountID,
		&value.Kind,
		&value.LanguageCode,
		&value.Value,
		&value.CreatedAt,
		&value.UpdatedAt,
	); err != nil {
		return bot.ProfileValue{}, err
	}

	return value, nil
}
