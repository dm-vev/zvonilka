package conversation

import (
	"context"
	"fmt"
	"strings"
)

// PublishUserUpdate appends one user-scoped sync event per active conversation membership.
func (s *Service) PublishUserUpdate(ctx context.Context, params PublishUserUpdateParams) ([]EventEnvelope, error) {
	if err := s.validateContext(ctx, "publish user update"); err != nil {
		return nil, err
	}

	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.PayloadType = strings.TrimSpace(params.PayloadType)
	params.Metadata = trimMetadata(params.Metadata)
	if params.AccountID == "" || params.PayloadType == "" {
		return nil, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var savedEvents []EventEnvelope
	err := s.runTx(ctx, func(tx Store) error {
		conversations, err := tx.ConversationsByAccountID(ctx, params.AccountID)
		if err != nil {
			return fmt.Errorf("load conversations for account %s: %w", params.AccountID, err)
		}
		if len(conversations) == 0 {
			savedEvents = nil
			return nil
		}

		savedEvents = make([]EventEnvelope, 0, len(conversations))
		for _, conversation := range conversations {
			metadata := map[string]string{
				"user_id": params.AccountID,
			}
			for key, value := range params.Metadata {
				metadata[key] = value
			}

			event, err := tx.SaveEvent(ctx, EventEnvelope{
				EventID:        eventID(),
				EventType:      EventTypeUserUpdated,
				ConversationID: conversation.ID,
				ActorAccountID: params.AccountID,
				ActorDeviceID:  params.DeviceID,
				PayloadType:    params.PayloadType,
				Metadata:       metadata,
				CreatedAt:      now,
			})
			if err != nil {
				return fmt.Errorf(
					"save user update event for account %s in conversation %s: %w",
					params.AccountID,
					conversation.ID,
					err,
				)
			}
			savedEvents = append(savedEvents, event)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return savedEvents, nil
}
