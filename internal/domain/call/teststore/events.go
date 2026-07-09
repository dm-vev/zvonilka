package teststore

import (
	"context"
	"slices"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

func (s *memoryStore) SaveEvent(_ context.Context, value call.Event) (call.Event, error) {
	value.EventID = strings.TrimSpace(value.EventID)
	value.CallID = strings.TrimSpace(value.CallID)
	value.ConversationID = strings.TrimSpace(value.ConversationID)
	value.ActorAccountID = strings.TrimSpace(value.ActorAccountID)
	value.ActorDeviceID = strings.TrimSpace(value.ActorDeviceID)
	if value.EventID == "" || value.CallID == "" || value.ConversationID == "" || value.EventType == call.EventTypeUnspecified {
		return call.Event{}, call.ErrInvalidInput
	}

	s.nextSequence++
	value.Sequence = s.nextSequence
	value.Metadata = cloneStringMap(value.Metadata)
	value.Call = call.Call{}
	s.eventsByID[value.EventID] = eventClone(value)
	s.eventOrder = append(s.eventOrder, value.EventID)
	return eventClone(value), nil
}

func (s *memoryStore) EventsAfterSequence(
	_ context.Context,
	fromSequence uint64,
	callID string,
	conversationID string,
	limit int,
) ([]call.Event, error) {
	callID = strings.TrimSpace(callID)
	conversationID = strings.TrimSpace(conversationID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]call.Event, 0)
	for _, eventID := range s.eventOrder {
		value := s.eventsByID[eventID]
		if value.Sequence <= fromSequence {
			continue
		}
		if callID != "" && value.CallID != callID {
			continue
		}
		if conversationID != "" && value.ConversationID != conversationID {
			continue
		}
		result = append(result, eventClone(value))
		if limit > 0 && len(result) >= limit {
			break
		}
	}

	slices.SortFunc(result, func(left call.Event, right call.Event) int {
		switch {
		case left.Sequence < right.Sequence:
			return -1
		case left.Sequence > right.Sequence:
			return 1
		default:
			return 0
		}
	})

	return result, nil
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
