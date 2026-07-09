package notificationworker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
	"github.com/dm-vev/zvonilka/internal/platform/config"
)

type webhookExecutor struct {
	client *http.Client
	url    string
}

type webhookDeliveryPayload struct {
	Delivery  webhookDeliveryView   `json:"delivery"`
	PushToken *webhookPushTokenView `json:"push_token,omitempty"`
}

type webhookDeliveryView struct {
	ID             string                        `json:"id"`
	DedupKey       string                        `json:"dedup_key"`
	EventID        string                        `json:"event_id"`
	ConversationID string                        `json:"conversation_id"`
	MessageID      string                        `json:"message_id"`
	AccountID      string                        `json:"account_id"`
	DeviceID       string                        `json:"device_id,omitempty"`
	PushTokenID    string                        `json:"push_token_id,omitempty"`
	Kind           notification.NotificationKind `json:"kind"`
	Reason         string                        `json:"reason"`
	Mode           notification.DeliveryMode     `json:"mode"`
	Priority       int                           `json:"priority"`
	Attempts       int                           `json:"attempts"`
	CreatedAt      string                        `json:"created_at"`
}

type webhookPushTokenView struct {
	ID        string `json:"id"`
	AccountID string `json:"account_id"`
	DeviceID  string `json:"device_id"`
	Provider  string `json:"provider"`
	Token     string `json:"token"`
	Platform  string `json:"platform"`
}

func newWebhookExecutor(cfg config.NotificationConfig) (notification.DeliveryExecutor, error) {
	if cfg.DeliveryWebhookURL == "" {
		return nil, nil
	}
	if cfg.DeliveryWebhookTimeout <= 0 {
		return nil, notification.ErrInvalidInput
	}

	return &webhookExecutor{
		client: &http.Client{Timeout: cfg.DeliveryWebhookTimeout},
		url:    cfg.DeliveryWebhookURL,
	}, nil
}

func (e *webhookExecutor) Deliver(
	ctx context.Context,
	delivery notification.Delivery,
	target notification.DeliveryTarget,
) error {
	payload, err := e.marshalPayload(delivery, target)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build notification webhook request for delivery %s: %w", delivery.ID, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver notification %s: %w", delivery.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fmt.Errorf("notification webhook returned %s", resp.Status)
	}

	return notification.PermanentDeliveryError(fmt.Errorf("notification webhook returned %s", resp.Status))
}

func (e *webhookExecutor) marshalPayload(
	delivery notification.Delivery,
	target notification.DeliveryTarget,
) ([]byte, error) {
	payload := webhookDeliveryPayload{
		Delivery: webhookDeliveryView{
			ID:             delivery.ID,
			DedupKey:       delivery.DedupKey,
			EventID:        delivery.EventID,
			ConversationID: delivery.ConversationID,
			MessageID:      delivery.MessageID,
			AccountID:      delivery.AccountID,
			DeviceID:       delivery.DeviceID,
			PushTokenID:    delivery.PushTokenID,
			Kind:           delivery.Kind,
			Reason:         delivery.Reason,
			Mode:           delivery.Mode,
			Priority:       delivery.Priority,
			Attempts:       delivery.Attempts + 1,
			CreatedAt:      delivery.CreatedAt.UTC().Format(time.RFC3339Nano),
		},
	}
	if delivery.Mode == notification.DeliveryModePush {
		if target.PushToken == nil {
			return nil, notification.PermanentDeliveryError(fmt.Errorf("push delivery %s is missing target token", delivery.ID))
		}
		payload.PushToken = &webhookPushTokenView{
			ID:        target.PushToken.ID,
			AccountID: target.PushToken.AccountID,
			DeviceID:  target.PushToken.DeviceID,
			Provider:  target.PushToken.Provider,
			Token:     target.PushToken.Token,
			Platform:  string(target.PushToken.Platform),
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal notification delivery %s: %w", delivery.ID, err)
	}

	return body, nil
}
