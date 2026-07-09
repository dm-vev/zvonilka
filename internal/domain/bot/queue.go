package bot

import (
	"context"
	"fmt"
	"time"
)

// GetUpdates returns queued bot updates using Telegram-like offset semantics.
func (s *Service) GetUpdates(ctx context.Context, params GetUpdatesParams) ([]Update, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return nil, err
	}

	webhook, err := s.store.WebhookByBotAccountID(ctx, account.ID)
	if err == nil && webhook.URL != "" {
		return nil, ErrConflict
	}
	if err != nil && err != ErrNotFound {
		return nil, fmt.Errorf("load webhook for bot %s: %w", account.ID, err)
	}

	if params.Offset > 0 {
		if err := s.store.DeleteUpdatesBefore(ctx, account.ID, params.Offset); err != nil {
			return nil, fmt.Errorf("acknowledge bot updates before %d: %w", params.Offset, err)
		}
	}

	limit := clampLimit(params.Limit, s.settings.GetUpdatesMaxLimit, s.settings.GetUpdatesMaxLimit)
	timeout := params.Timeout
	if timeout < 0 {
		timeout = 0
	}
	if timeout > s.settings.LongPollMaxTimeout {
		timeout = s.settings.LongPollMaxTimeout
	}

	deadline := s.currentTime().Add(timeout)
	for {
		entries, err := s.store.PendingUpdates(ctx, account.ID, 0, params.AllowedUpdates, time.Time{}, limit)
		if err != nil {
			return nil, fmt.Errorf("load bot updates: %w", err)
		}
		if len(entries) > 0 || timeout <= 0 || !s.currentTime().Before(deadline) {
			return updates(entries), nil
		}

		wait := s.settings.LongPollStep
		remaining := time.Until(deadline)
		if remaining < wait {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// SetWebhook stores one webhook configuration for a bot.
func (s *Service) SetWebhook(ctx context.Context, params SetWebhookParams) (WebhookInfo, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return WebhookInfo{}, err
	}
	if params.URL == "" {
		return WebhookInfo{}, ErrInvalidInput
	}

	now := s.currentTime()
	err = s.runTx(ctx, func(tx Store) error {
		if params.DropPendingUpdates {
			if err := tx.DeleteAllUpdates(ctx, account.ID); err != nil {
				return fmt.Errorf("drop pending bot updates: %w", err)
			}
		}

		_, err := tx.SaveWebhook(ctx, Webhook{
			BotAccountID:   account.ID,
			URL:            params.URL,
			SecretToken:    params.SecretToken,
			AllowedUpdates: params.AllowedUpdates,
			MaxConnections: params.MaxConnections,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		if err != nil {
			return fmt.Errorf("save bot webhook: %w", err)
		}

		return nil
	})
	if err != nil {
		return WebhookInfo{}, err
	}

	return s.WebhookInfo(ctx, WebhookInfoParams{BotToken: params.BotToken})
}

// DeleteWebhook removes one bot webhook configuration.
func (s *Service) DeleteWebhook(ctx context.Context, params DeleteWebhookParams) error {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return err
	}

	return s.runTx(ctx, func(tx Store) error {
		if params.DropPendingUpdates {
			if err := tx.DeleteAllUpdates(ctx, account.ID); err != nil {
				return fmt.Errorf("drop pending bot updates: %w", err)
			}
		}
		if err := tx.DeleteWebhook(ctx, account.ID); err != nil && err != ErrNotFound {
			return fmt.Errorf("delete bot webhook: %w", err)
		}

		return nil
	})
}

// WebhookInfo resolves the current webhook info for a bot.
func (s *Service) WebhookInfo(ctx context.Context, params WebhookInfoParams) (WebhookInfo, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return WebhookInfo{}, err
	}

	webhook, webhookErr := s.store.WebhookByBotAccountID(ctx, account.ID)
	if webhookErr != nil && webhookErr != ErrNotFound {
		return WebhookInfo{}, fmt.Errorf("load bot webhook info: %w", webhookErr)
	}

	pending, err := s.store.PendingUpdateCount(ctx, account.ID)
	if err != nil {
		return WebhookInfo{}, fmt.Errorf("count pending bot updates: %w", err)
	}
	if webhookErr == ErrNotFound {
		return webhookInfo(Webhook{}, pending), nil
	}

	return webhookInfo(webhook, pending), nil
}

func updates(entries []QueueEntry) []Update {
	result := make([]Update, 0, len(entries))
	for _, entry := range entries {
		payload := entry.Payload
		payload.UpdateID = entry.UpdateID
		result = append(result, payload)
	}

	return result
}
