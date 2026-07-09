package notification

import (
	"context"
	"time"
)

// Store persists notification state.
type Store interface {
	WithinTx(ctx context.Context, fn func(Store) error) error

	SavePreference(ctx context.Context, preference Preference) (Preference, error)
	PreferenceByAccountID(ctx context.Context, accountID string) (Preference, error)

	SaveOverride(ctx context.Context, override ConversationOverride) (ConversationOverride, error)
	OverrideByConversationAndAccount(ctx context.Context, conversationID string, accountID string) (ConversationOverride, error)

	SavePushToken(ctx context.Context, token PushToken) (PushToken, error)
	PushTokenByID(ctx context.Context, tokenID string) (PushToken, error)
	PushTokensByAccountID(ctx context.Context, accountID string) ([]PushToken, error)
	DeletePushToken(ctx context.Context, tokenID string) error

	SaveDelivery(ctx context.Context, delivery Delivery) (Delivery, error)
	DeliveryByID(ctx context.Context, deliveryID string) (Delivery, error)
	DeliveriesDue(ctx context.Context, before time.Time, limit int) ([]Delivery, error)
	ClaimDeliveries(ctx context.Context, params ClaimDeliveriesParams) ([]Delivery, error)
	MarkDeliveryDelivered(ctx context.Context, params MarkDeliveryDeliveredParams) (Delivery, error)
	FailDelivery(ctx context.Context, params FailDeliveryParams) (Delivery, error)
	RetryDelivery(ctx context.Context, params RetryDeliveryParams) (Delivery, error)

	SaveWorkerCursor(ctx context.Context, cursor WorkerCursor) (WorkerCursor, error)
	WorkerCursorByName(ctx context.Context, name string) (WorkerCursor, error)
}
