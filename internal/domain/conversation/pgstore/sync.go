package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveSyncState inserts or updates a device sync-state row.
func (s *Store) SaveSyncState(ctx context.Context, state conversation.SyncState) (conversation.SyncState, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.SyncState{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.SyncState{}, err
	}
	if state.DeviceID == "" {
		return conversation.SyncState{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveSyncState(ctx, state)
	}

	var saved conversation.SyncState
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveSyncState(ctx, state)
		return saveErr
	})
	if err != nil {
		return conversation.SyncState{}, err
	}

	return saved, nil
}

func (s *Store) saveSyncState(ctx context.Context, state conversation.SyncState) (conversation.SyncState, error) {
	state.DeviceID = strings.TrimSpace(state.DeviceID)
	state.AccountID = strings.TrimSpace(state.AccountID)
	if state.DeviceID == "" {
		return conversation.SyncState{}, conversation.ErrInvalidInput
	}

	watermarks, err := encodeWatermarks(state.ConversationWatermarks)
	if err != nil {
		return conversation.SyncState{}, fmt.Errorf("encode sync watermarks: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	device_id, account_id, last_applied_sequence, last_acked_sequence, conversation_watermarks, server_time, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (device_id) DO UPDATE SET
	account_id = EXCLUDED.account_id,
	last_applied_sequence = EXCLUDED.last_applied_sequence,
	last_acked_sequence = EXCLUDED.last_acked_sequence,
	conversation_watermarks = EXCLUDED.conversation_watermarks,
	server_time = EXCLUDED.server_time,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("conversation_sync_states"), syncStateColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		state.DeviceID,
		state.AccountID,
		state.LastAppliedSequence,
		state.LastAckedSequence,
		watermarks,
		state.ServerTime.UTC(),
		state.ServerTime.UTC(),
	)

	saved, err := scanSyncState(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.SyncState{}, mappedErr
		}
		return conversation.SyncState{}, fmt.Errorf("save sync state %s: %w", state.DeviceID, err)
	}

	return saved, nil
}

// SyncStateByDevice resolves a device sync-state row.
func (s *Store) SyncStateByDevice(ctx context.Context, deviceID string) (conversation.SyncState, error) {
	if err := s.requireStore(); err != nil {
		return conversation.SyncState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.SyncState{}, err
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return conversation.SyncState{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE device_id = $1`, syncStateColumnList, s.table("conversation_sync_states"))
	row := s.conn().QueryRowContext(ctx, query, deviceID)
	state, err := scanSyncState(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.SyncState{}, conversation.ErrNotFound
		}
		return conversation.SyncState{}, fmt.Errorf("load sync state %s: %w", deviceID, err)
	}

	return state, nil
}
