package bot

import (
	"context"
	"time"
)

// Store persists bot webhook and update state.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SaveWebhook(ctx context.Context, webhook Webhook) (Webhook, error)
	WebhookByBotAccountID(ctx context.Context, botAccountID string) (Webhook, error)
	ListWebhooks(ctx context.Context) ([]Webhook, error)
	DeleteWebhook(ctx context.Context, botAccountID string) error

	SaveUpdate(ctx context.Context, entry QueueEntry) (QueueEntry, error)
	PendingUpdates(
		ctx context.Context,
		botAccountID string,
		offset int64,
		allowed []UpdateType,
		before time.Time,
		limit int,
	) ([]QueueEntry, error)
	DeleteUpdatesBefore(ctx context.Context, botAccountID string, offset int64) error
	DeleteUpdate(ctx context.Context, botAccountID string, updateID int64) error
	DeleteAllUpdates(ctx context.Context, botAccountID string) error
	PendingUpdateCount(ctx context.Context, botAccountID string) (int, error)
	RetryUpdate(ctx context.Context, params RetryParams) (QueueEntry, error)

	SaveCursor(ctx context.Context, cursor Cursor) (Cursor, error)
	CursorByName(ctx context.Context, name string) (Cursor, error)
}
