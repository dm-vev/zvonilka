package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

// SaveDelivery inserts or updates a delivery hint.
func (s *Store) SaveDelivery(ctx context.Context, delivery notification.Delivery) (notification.Delivery, error) {
	if err := s.requireStore(); err != nil {
		return notification.Delivery{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.Delivery{}, err
	}
	if s.tx != nil {
		return s.saveDelivery(ctx, delivery)
	}

	var saved notification.Delivery
	err := s.WithinTx(ctx, func(tx notification.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveDelivery(ctx, delivery)
		return saveErr
	})
	if err != nil {
		return notification.Delivery{}, err
	}

	return saved, nil
}

func (s *Store) saveDelivery(ctx context.Context, delivery notification.Delivery) (notification.Delivery, error) {
	now := time.Now().UTC()
	delivery, err := notification.NormalizeDelivery(delivery, now)
	if err != nil {
		return notification.Delivery{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id,
	dedup_key,
	event_id,
	conversation_id,
	message_id,
	account_id,
	device_id,
	push_token_id,
	kind,
	reason,
	mode,
	state,
	priority,
	attempts,
	next_attempt_at,
	last_attempt_at,
	last_error,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
)
ON CONFLICT (dedup_key) DO UPDATE SET
	event_id = EXCLUDED.event_id,
	conversation_id = EXCLUDED.conversation_id,
	message_id = EXCLUDED.message_id,
	account_id = EXCLUDED.account_id,
	device_id = EXCLUDED.device_id,
	push_token_id = EXCLUDED.push_token_id,
	kind = EXCLUDED.kind,
	reason = EXCLUDED.reason,
	mode = EXCLUDED.mode,
	state = CASE
		WHEN EXCLUDED.attempts > %s.attempts THEN EXCLUDED.state
		ELSE %s.state
	END,
	priority = CASE
		WHEN EXCLUDED.attempts > %s.attempts THEN EXCLUDED.priority
		ELSE %s.priority
	END,
	attempts = GREATEST(%s.attempts, EXCLUDED.attempts),
	next_attempt_at = CASE
		WHEN EXCLUDED.attempts > %s.attempts THEN EXCLUDED.next_attempt_at
		ELSE %s.next_attempt_at
	END,
	last_attempt_at = CASE
		WHEN EXCLUDED.attempts > %s.attempts THEN EXCLUDED.last_attempt_at
		ELSE %s.last_attempt_at
	END,
	last_error = CASE
		WHEN EXCLUDED.attempts > %s.attempts THEN EXCLUDED.last_error
		ELSE %s.last_error
	END,
	updated_at = CASE
		WHEN EXCLUDED.attempts > %s.attempts THEN EXCLUDED.updated_at
		ELSE %s.updated_at
	END
RETURNING id, dedup_key, event_id, conversation_id, message_id, account_id, device_id, push_token_id,
	kind, reason, mode, state, priority, attempts, next_attempt_at, last_attempt_at, last_error, created_at, updated_at
`, s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
		s.table("notification_deliveries"),
	)

	row := s.conn().QueryRowContext(
		ctx,
		query,
		delivery.ID,
		delivery.DedupKey,
		delivery.EventID,
		delivery.ConversationID,
		delivery.MessageID,
		delivery.AccountID,
		delivery.DeviceID,
		delivery.PushTokenID,
		delivery.Kind,
		delivery.Reason,
		delivery.Mode,
		delivery.State,
		delivery.Priority,
		delivery.Attempts,
		delivery.NextAttemptAt.UTC(),
		encodeTime(delivery.LastAttemptAt),
		delivery.LastError,
		delivery.CreatedAt.UTC(),
		delivery.UpdatedAt.UTC(),
	)

	saved, err := scanDelivery(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.Delivery{}, notification.ErrNotFound
		}
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return notification.Delivery{}, mappedErr
		}

		return notification.Delivery{}, fmt.Errorf("save delivery %s: %w", delivery.DedupKey, err)
	}

	return saved, nil
}

// DeliveryByID resolves one delivery by primary key.
func (s *Store) DeliveryByID(ctx context.Context, deliveryID string) (notification.Delivery, error) {
	if err := s.requireStore(); err != nil {
		return notification.Delivery{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.Delivery{}, err
	}
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return notification.Delivery{}, notification.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT id, dedup_key, event_id, conversation_id, message_id, account_id, device_id, push_token_id,
	kind, reason, mode, state, priority, attempts, next_attempt_at, last_attempt_at, last_error, created_at, updated_at
FROM %s
WHERE id = $1
`, s.table("notification_deliveries"))

	row := s.conn().QueryRowContext(ctx, query, deliveryID)
	delivery, err := scanDelivery(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.Delivery{}, notification.ErrNotFound
		}
		return notification.Delivery{}, fmt.Errorf("load delivery %s: %w", deliveryID, err)
	}

	return delivery, nil
}

// DeliveriesDue returns deliveries ready for retry.
func (s *Store) DeliveriesDue(ctx context.Context, before time.Time, limit int) ([]notification.Delivery, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}

	query := fmt.Sprintf(`
SELECT id, dedup_key, event_id, conversation_id, message_id, account_id, device_id, push_token_id,
	kind, reason, mode, state, priority, attempts, next_attempt_at, last_attempt_at, last_error, created_at, updated_at
FROM %s
WHERE state = $1 AND next_attempt_at <= $2
ORDER BY priority DESC, next_attempt_at ASC, created_at ASC, id ASC
LIMIT $3
`, s.table("notification_deliveries"))

	rows, err := s.conn().QueryContext(ctx, query, notification.DeliveryStateQueued, before.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("load deliveries due: %w", err)
	}
	defer rows.Close()

	deliveries := make([]notification.Delivery, 0, limit)
	for rows.Next() {
		delivery, scanErr := scanDelivery(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan due delivery: %w", scanErr)
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due deliveries: %w", err)
	}

	return deliveries, nil
}

// RetryDelivery schedules another attempt for an existing delivery.
func (s *Store) RetryDelivery(ctx context.Context, params notification.RetryDeliveryParams) (notification.Delivery, error) {
	if err := s.requireStore(); err != nil {
		return notification.Delivery{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.Delivery{}, err
	}
	params.DeliveryID = strings.TrimSpace(params.DeliveryID)
	params.LastError = strings.TrimSpace(params.LastError)
	if params.DeliveryID == "" {
		return notification.Delivery{}, notification.ErrInvalidInput
	}

	delivery, err := s.DeliveryByID(ctx, params.DeliveryID)
	if err != nil {
		return notification.Delivery{}, err
	}

	now := time.Now().UTC()
	delivery.Attempts++
	delivery.LastError = params.LastError
	delivery.LastAttemptAt = now
	if params.RetryAt.IsZero() {
		delivery.NextAttemptAt = now
	} else {
		delivery.NextAttemptAt = params.RetryAt.UTC()
	}
	delivery.State = notification.DeliveryStateQueued
	delivery.UpdatedAt = now

	return s.saveDelivery(ctx, delivery)
}
