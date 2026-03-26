package call

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func trimMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}

	return result
}

func (s *Service) appendEvent(
	ctx context.Context,
	store Store,
	call Call,
	eventType EventType,
	actorAccountID string,
	actorDeviceID string,
	metadata map[string]string,
	createdAt time.Time,
) (Event, error) {
	if createdAt.IsZero() {
		createdAt = s.currentTime()
	}

	event, err := store.SaveEvent(ctx, Event{
		EventID:        eventID(),
		CallID:         call.ID,
		ConversationID: call.ConversationID,
		EventType:      eventType,
		ActorAccountID: strings.TrimSpace(actorAccountID),
		ActorDeviceID:  strings.TrimSpace(actorDeviceID),
		Metadata:       trimMetadata(metadata),
		CreatedAt:      createdAt,
	})
	if err != nil {
		return Event{}, fmt.Errorf("save call event %s for %s: %w", eventType, call.ID, err)
	}

	return event, nil
}

func eventID() string {
	value, err := newID("evt")
	if err != nil {
		return "evt"
	}

	return value
}
