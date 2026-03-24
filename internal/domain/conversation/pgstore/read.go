package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveReadState inserts or updates a read-state row.
func (s *Store) SaveReadState(ctx context.Context, state conversation.ReadState) (conversation.ReadState, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ReadState{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ReadState{}, err
	}
	if state.ConversationID == "" || state.AccountID == "" || state.DeviceID == "" {
		return conversation.ReadState{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveReadState(ctx, state)
	}

	var saved conversation.ReadState
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveReadState(ctx, state)
		return saveErr
	})
	if err != nil {
		return conversation.ReadState{}, err
	}

	return saved, nil
}

func (s *Store) saveReadState(ctx context.Context, state conversation.ReadState) (conversation.ReadState, error) {
	state.ConversationID = strings.TrimSpace(state.ConversationID)
	state.AccountID = strings.TrimSpace(state.AccountID)
	state.DeviceID = strings.TrimSpace(state.DeviceID)
	if state.ConversationID == "" || state.AccountID == "" || state.DeviceID == "" {
		return conversation.ReadState{}, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	conversation_id, account_id, device_id, last_read_sequence, last_delivered_sequence, last_acked_sequence, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (conversation_id, account_id, device_id) DO UPDATE SET
	last_read_sequence = EXCLUDED.last_read_sequence,
	last_delivered_sequence = EXCLUDED.last_delivered_sequence,
	last_acked_sequence = EXCLUDED.last_acked_sequence,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("conversation_read_states"), readStateColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		state.ConversationID,
		state.AccountID,
		state.DeviceID,
		state.LastReadSequence,
		state.LastDeliveredSequence,
		state.LastAckedSequence,
		state.UpdatedAt.UTC(),
	)

	saved, err := scanReadState(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.ReadState{}, mappedErr
		}
		return conversation.ReadState{}, fmt.Errorf("save read state %s/%s/%s: %w", state.ConversationID, state.AccountID, state.DeviceID, err)
	}

	return saved, nil
}

// ReadStateByConversationAndDevice resolves a read-state row.
func (s *Store) ReadStateByConversationAndDevice(ctx context.Context, conversationID string, deviceID string) (conversation.ReadState, error) {
	if err := s.requireStore(); err != nil {
		return conversation.ReadState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.ReadState{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	deviceID = strings.TrimSpace(deviceID)
	if conversationID == "" || deviceID == "" {
		return conversation.ReadState{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE conversation_id = $1 AND device_id = $2`, readStateColumnList, s.table("conversation_read_states"))
	row := s.conn().QueryRowContext(ctx, query, conversationID, deviceID)
	state, err := scanReadState(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ReadState{}, conversation.ErrNotFound
		}
		return conversation.ReadState{}, fmt.Errorf("load read state %s/%s: %w", conversationID, deviceID, err)
	}

	return state, nil
}

// ReadStatesByDevice lists all read-state rows for a device.
func (s *Store) ReadStatesByDevice(ctx context.Context, deviceID string) ([]conversation.ReadState, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE device_id = $1 ORDER BY conversation_id ASC`, readStateColumnList, s.table("conversation_read_states"))
	rows, err := s.conn().QueryContext(ctx, query, deviceID)
	if err != nil {
		return nil, fmt.Errorf("list read states for device %s: %w", deviceID, err)
	}
	defer rows.Close()

	states := make([]conversation.ReadState, 0)
	for rows.Next() {
		state, scanErr := scanReadState(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan read state for device %s: %w", deviceID, scanErr)
		}
		states = append(states, state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate read states for device %s: %w", deviceID, err)
	}

	return states, nil
}
