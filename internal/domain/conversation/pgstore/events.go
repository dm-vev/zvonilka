package pgstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveEvent inserts a sync event and returns the generated sequence.
func (s *Store) SaveEvent(ctx context.Context, event conversation.EventEnvelope) (conversation.EventEnvelope, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.EventEnvelope{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.EventEnvelope{}, err
	}
	if event.EventID == "" {
		return conversation.EventEnvelope{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveEvent(ctx, event)
	}

	var saved conversation.EventEnvelope
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveEvent(ctx, event)
		return saveErr
	})
	if err != nil {
		return conversation.EventEnvelope{}, err
	}

	return saved, nil
}

func (s *Store) saveEvent(ctx context.Context, event conversation.EventEnvelope) (conversation.EventEnvelope, error) {
	event.EventID = strings.TrimSpace(event.EventID)
	event.EventType = conversation.EventType(strings.TrimSpace(string(event.EventType)))
	event.ConversationID = strings.TrimSpace(event.ConversationID)
	event.ActorAccountID = strings.TrimSpace(event.ActorAccountID)
	event.ActorDeviceID = strings.TrimSpace(event.ActorDeviceID)
	event.CausationID = strings.TrimSpace(event.CausationID)
	event.CorrelationID = strings.TrimSpace(event.CorrelationID)
	event.PayloadType = strings.TrimSpace(event.PayloadType)
	if event.EventID == "" || event.EventType == conversation.EventTypeUnspecified || event.ConversationID == "" || event.ActorAccountID == "" {
		return conversation.EventEnvelope{}, conversation.ErrInvalidInput
	}

	payloadKeyID, payloadAlgorithm, payloadNonce, payloadCiphertext, payloadAAD, payloadMetadata, err := encodePayload(event.Payload)
	if err != nil {
		return conversation.EventEnvelope{}, fmt.Errorf("encode event payload: %w", err)
	}
	metadata, err := encodeMetadata(event.Metadata)
	if err != nil {
		return conversation.EventEnvelope{}, fmt.Errorf("encode event metadata: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	event_id, event_type, conversation_id, actor_account_id, actor_device_id, causation_id, correlation_id,
	message_id, payload_type, payload_key_id, payload_algorithm, payload_nonce, payload_ciphertext, payload_aad,
	payload_metadata, read_through_sequence, metadata, created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14,
	$15, $16, $17, $18
)
RETURNING %s
`, s.table("conversation_events"), eventColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		event.EventID,
		event.EventType,
		event.ConversationID,
		event.ActorAccountID,
		nullString(event.ActorDeviceID),
		event.CausationID,
		event.CorrelationID,
		nullString(event.MessageID),
		event.PayloadType,
		payloadKeyID,
		payloadAlgorithm,
		payloadNonce,
		payloadCiphertext,
		payloadAAD,
		payloadMetadata,
		event.ReadThroughSequence,
		metadata,
		event.CreatedAt.UTC(),
	)

	saved, err := scanEvent(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.EventEnvelope{}, mappedErr
		}
		return conversation.EventEnvelope{}, fmt.Errorf("save event %s: %w", event.EventID, err)
	}

	return saved, nil
}

// EventsAfterSequence lists events after the requested sequence.
func (s *Store) EventsAfterSequence(ctx context.Context, fromSequence uint64, limit int, conversationIDs []string) ([]conversation.EventEnvelope, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	baseQuery := fmt.Sprintf(`SELECT %s FROM %s WHERE sequence > $1`, eventColumnList, s.table("conversation_events"))
	args := []any{fromSequence}
	if len(conversationIDs) > 0 {
		queryIDs := make([]string, 0, len(conversationIDs))
		for _, conversationID := range conversationIDs {
			conversationID = strings.TrimSpace(conversationID)
			if conversationID != "" {
				queryIDs = append(queryIDs, conversationID)
			}
		}
		if len(queryIDs) > 0 {
			baseQuery += " AND conversation_id = ANY($2)"
			args = append(args, queryIDs)
		}
	}
	baseQuery += " ORDER BY sequence ASC"
	if limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.conn().QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("list events after %d: %w", fromSequence, err)
	}
	defer rows.Close()

	events := make([]conversation.EventEnvelope, 0)
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan event after %d: %w", fromSequence, scanErr)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events after %d: %w", fromSequence, err)
	}

	return events, nil
}
