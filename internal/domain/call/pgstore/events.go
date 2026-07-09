package pgstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

const eventColumnList = `
event_id, call_id, conversation_id, event_type, actor_account_id, actor_device_id, sequence, metadata, created_at
`

func (s *Store) SaveEvent(ctx context.Context, value call.Event) (call.Event, error) {
	if err := s.requireStore(); err != nil {
		return call.Event{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Event{}, err
	}
	if s.tx != nil {
		return s.saveEvent(ctx, value)
	}

	var saved call.Event
	err := s.WithinTx(ctx, func(tx call.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveEvent(ctx, value)
		return saveErr
	})
	if err != nil {
		return call.Event{}, err
	}
	return saved, nil
}

func (s *Store) saveEvent(ctx context.Context, value call.Event) (call.Event, error) {
	value.EventID = strings.TrimSpace(value.EventID)
	value.CallID = strings.TrimSpace(value.CallID)
	value.ConversationID = strings.TrimSpace(value.ConversationID)
	value.ActorAccountID = strings.TrimSpace(value.ActorAccountID)
	value.ActorDeviceID = strings.TrimSpace(value.ActorDeviceID)
	if value.EventID == "" || value.CallID == "" || value.ConversationID == "" || value.EventType == call.EventTypeUnspecified {
		return call.Event{}, call.ErrInvalidInput
	}

	metadata, err := encodeMetadata(value.Metadata)
	if err != nil {
		return call.Event{}, fmt.Errorf("encode event metadata: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	event_id, call_id, conversation_id, event_type, actor_account_id, actor_device_id, metadata, created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING %s
`, s.table("call_events"), eventColumnList)
	row := s.conn().QueryRowContext(
		ctx,
		query,
		value.EventID,
		value.CallID,
		value.ConversationID,
		value.EventType,
		nullString(value.ActorAccountID),
		nullString(value.ActorDeviceID),
		metadata,
		value.CreatedAt.UTC(),
	)
	saved, err := scanEvent(row)
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return call.Event{}, mapped
		}
		return call.Event{}, fmt.Errorf("save call event %s: %w", value.EventID, err)
	}
	return saved, nil
}

func (s *Store) EventsAfterSequence(
	ctx context.Context,
	fromSequence uint64,
	callID string,
	conversationID string,
	limit int,
) ([]call.Event, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE sequence > $1`, eventColumnList, s.table("call_events"))
	args := []any{fromSequence}
	if strings.TrimSpace(callID) != "" {
		query += ` AND call_id = $2`
		args = append(args, strings.TrimSpace(callID))
	} else if strings.TrimSpace(conversationID) != "" {
		query += ` AND conversation_id = $2`
		args = append(args, strings.TrimSpace(conversationID))
	}
	query += ` ORDER BY sequence ASC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list call events: %w", err)
	}
	defer rows.Close()

	result := make([]call.Event, 0)
	for rows.Next() {
		value, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan call event: %w", scanErr)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate call events: %w", err)
	}
	return result, nil
}
