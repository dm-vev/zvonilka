package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
)

const defaultWorkerName = "conversation_notifications"

// Worker fans out conversation events into notification delivery hints.
type Worker struct {
	service       *Service
	conversations conversation.Store
	presence      *presence.Service
	name          string
	now           func() time.Time
}

// NewWorker constructs a notification worker.
func NewWorker(service *Service, conversations conversation.Store, presenceService *presence.Service) (*Worker, error) {
	if service == nil || conversations == nil || presenceService == nil {
		return nil, ErrInvalidInput
	}

	return &Worker{
		service:       service,
		conversations: conversations,
		presence:      presenceService,
		name:          defaultWorkerName,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

// Run polls the conversation event log and enqueues notification deliveries.
func (w *Worker) Run(ctx context.Context, logger *slog.Logger) error {
	if err := w.validateContext(ctx, "run notification worker"); err != nil {
		return err
	}
	if logger == nil {
		return ErrInvalidInput
	}

	interval := w.service.settings.WorkerPollInterval
	if interval <= 0 {
		interval = DefaultSettings().WorkerPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if err := w.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "process notification fanout batch", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) error {
	cursor, err := w.service.WorkerCursorByName(ctx, w.name)
	if err != nil {
		return err
	}

	events, err := w.conversations.EventsAfterSequence(ctx, cursor.LastSequence, w.service.settings.BatchSize, nil)
	if err != nil {
		return fmt.Errorf("load conversation events after %d: %w", cursor.LastSequence, err)
	}
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		now := w.currentTime()
		err = w.service.store.WithinTx(ctx, func(tx Store) error {
			if event.EventType == conversation.EventTypeMessageCreated {
				deliveries, buildErr := w.buildDeliveriesForEvent(ctx, event)
				if buildErr != nil {
					return buildErr
				}
				for _, delivery := range deliveries {
					if _, saveErr := tx.SaveDelivery(ctx, delivery); saveErr != nil {
						return fmt.Errorf("save notification delivery %s: %w", delivery.DedupKey, saveErr)
					}
				}
			}

			cursor.LastSequence = maxUint64(cursor.LastSequence, event.Sequence)
			cursor.UpdatedAt = now
			if _, saveErr := tx.SaveWorkerCursor(ctx, cursor); saveErr != nil {
				return fmt.Errorf("save notification cursor %s: %w", cursor.Name, saveErr)
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *Worker) buildDeliveriesForEvent(ctx context.Context, event conversation.EventEnvelope) ([]Delivery, error) {
	conversationID := event.ConversationID
	if conversationID == "" || event.MessageID == "" {
		return nil, nil
	}

	conversationRecord, err := w.conversations.ConversationByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("load conversation %s: %w", conversationID, err)
	}

	message, err := w.conversations.MessageByID(ctx, conversationID, event.MessageID)
	if err != nil {
		return nil, fmt.Errorf("load message %s in conversation %s: %w", event.MessageID, conversationID, err)
	}

	members, err := w.conversations.ConversationMembersByConversationID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("load members for conversation %s: %w", conversationID, err)
	}

	preferenceByAccountID := make(map[string]Preference, len(members))
	overrideByAccountID := make(map[string]ConversationOverride, len(members))
	presenceByAccountID := make(map[string]presence.Snapshot, len(members))
	pushTokensByAccountID := make(map[string][]PushToken, len(members))

	for _, member := range members {
		if !activeConversationMember(member) || member.AccountID == message.SenderAccountID {
			continue
		}

		account, accountErr := w.service.identity.AccountByID(ctx, member.AccountID)
		if accountErr != nil {
			if errors.Is(accountErr, identity.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("load account %s: %w", member.AccountID, accountErr)
		}
		if account.Status != identity.AccountStatusActive {
			continue
		}

		preference, prefErr := w.service.PreferenceByAccountID(ctx, member.AccountID)
		if prefErr != nil {
			return nil, prefErr
		}
		preferenceByAccountID[member.AccountID] = preference

		override, overrideErr := w.service.ConversationOverrideByConversationAndAccount(ctx, conversationID, member.AccountID)
		if overrideErr != nil {
			return nil, overrideErr
		}
		overrideByAccountID[member.AccountID] = override

		presenceSnapshot, presenceErr := w.presence.GetPresence(ctx, presence.GetParams{
			AccountID:       member.AccountID,
			ViewerAccountID: member.AccountID,
		})
		if presenceErr != nil {
			if errors.Is(presenceErr, presence.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("resolve presence for account %s: %w", member.AccountID, presenceErr)
		}
		presenceByAccountID[member.AccountID] = presenceSnapshot

		tokens, tokenErr := w.service.PushTokensByAccountID(ctx, member.AccountID)
		if tokenErr != nil {
			return nil, tokenErr
		}
		pushTokensByAccountID[member.AccountID] = tokens
	}

	deliveries := buildDeliveries(
		w.currentTime(),
		event.EventID,
		conversationRecord,
		message,
		members,
		preferenceByAccountID,
		overrideByAccountID,
		presenceByAccountID,
		pushTokensByAccountID,
	)

	return deliveries, nil
}

func (w *Worker) currentTime() time.Time {
	if w == nil || w.now == nil {
		return time.Now().UTC()
	}

	return w.now().UTC()
}

func (w *Worker) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func maxUint64(values ...uint64) uint64 {
	var max uint64
	for _, value := range values {
		if value > max {
			max = value
		}
	}

	return max
}
