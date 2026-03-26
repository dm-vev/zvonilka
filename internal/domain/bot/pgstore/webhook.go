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

// SaveWebhook inserts or updates a bot webhook row.
func (s *Store) SaveWebhook(ctx context.Context, webhook bot.Webhook) (bot.Webhook, error) {
	if err := s.requireStore(); err != nil {
		return bot.Webhook{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.Webhook{}, err
	}
	if s.tx != nil {
		return s.saveWebhook(ctx, webhook)
	}

	var saved bot.Webhook
	err := s.WithinTx(ctx, func(tx bot.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveWebhook(ctx, webhook)
		return saveErr
	})
	if err != nil {
		return bot.Webhook{}, err
	}

	return saved, nil
}

func (s *Store) saveWebhook(ctx context.Context, webhook bot.Webhook) (bot.Webhook, error) {
	now := time.Now().UTC()
	webhook, err := bot.NormalizeWebhook(webhook, now)
	if err != nil {
		return bot.Webhook{}, err
	}
	allowed, err := encodeAllowedUpdates(webhook.AllowedUpdates)
	if err != nil {
		return bot.Webhook{}, fmt.Errorf("encode allowed updates: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	bot_account_id,
	url,
	secret_token,
	allowed_updates,
	max_connections,
	last_error_message,
	last_error_at,
	last_success_at,
	created_at,
	updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (bot_account_id) DO UPDATE SET
	url = EXCLUDED.url,
	secret_token = EXCLUDED.secret_token,
	allowed_updates = EXCLUDED.allowed_updates,
	max_connections = EXCLUDED.max_connections,
	last_error_message = EXCLUDED.last_error_message,
	last_error_at = EXCLUDED.last_error_at,
	last_success_at = EXCLUDED.last_success_at,
	updated_at = EXCLUDED.updated_at
RETURNING bot_account_id, url, secret_token, allowed_updates, max_connections,
	last_error_message, last_error_at, last_success_at, created_at, updated_at
`, s.table("bot_webhooks"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		webhook.BotAccountID,
		webhook.URL,
		webhook.SecretToken,
		allowed,
		webhook.MaxConnections,
		webhook.LastErrorMessage,
		encodeTime(webhook.LastErrorAt),
		encodeTime(webhook.LastSuccessAt),
		webhook.CreatedAt.UTC(),
		webhook.UpdatedAt.UTC(),
	)

	saved, err := scanWebhook(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.Webhook{}, bot.ErrNotFound
		}
		if mapped := mapConstraintError(err); mapped != nil {
			return bot.Webhook{}, mapped
		}

		return bot.Webhook{}, fmt.Errorf("save bot webhook for %s: %w", webhook.BotAccountID, err)
	}

	return saved, nil
}

// WebhookByBotAccountID resolves one webhook row by bot account.
func (s *Store) WebhookByBotAccountID(ctx context.Context, botAccountID string) (bot.Webhook, error) {
	if err := s.requireStore(); err != nil {
		return bot.Webhook{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return bot.Webhook{}, err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return bot.Webhook{}, bot.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT bot_account_id, url, secret_token, allowed_updates, max_connections,
	last_error_message, last_error_at, last_success_at, created_at, updated_at
FROM %s
WHERE bot_account_id = $1
`, s.table("bot_webhooks"))

	row := s.conn().QueryRowContext(ctx, query, botAccountID)
	webhook, err := scanWebhook(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return bot.Webhook{}, bot.ErrNotFound
		}
		return bot.Webhook{}, fmt.Errorf("load bot webhook for %s: %w", botAccountID, err)
	}

	return webhook, nil
}

// ListWebhooks returns all configured bot webhooks.
func (s *Store) ListWebhooks(ctx context.Context) ([]bot.Webhook, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
SELECT bot_account_id, url, secret_token, allowed_updates, max_connections,
	last_error_message, last_error_at, last_success_at, created_at, updated_at
FROM %s
ORDER BY bot_account_id ASC
`, s.table("bot_webhooks"))

	rows, err := s.conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list bot webhooks: %w", err)
	}
	defer rows.Close()

	result := make([]bot.Webhook, 0)
	for rows.Next() {
		webhook, err := scanWebhook(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bot webhook: %w", err)
		}
		result = append(result, webhook)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bot webhooks: %w", err)
	}

	return result, nil
}

// DeleteWebhook removes one webhook row.
func (s *Store) DeleteWebhook(ctx context.Context, botAccountID string) error {
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

	query := fmt.Sprintf(`DELETE FROM %s WHERE bot_account_id = $1`, s.table("bot_webhooks"))
	result, err := s.conn().ExecContext(ctx, query, botAccountID)
	if err != nil {
		return fmt.Errorf("delete bot webhook for %s: %w", botAccountID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bot webhook for %s: rows affected: %w", botAccountID, err)
	}
	if rows == 0 {
		return bot.ErrNotFound
	}

	return nil
}
