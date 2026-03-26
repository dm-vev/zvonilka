package pgstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

const callColumnList = `
call_id, conversation_id, initiator_account_id, active_session_id, requested_video,
state, end_reason, started_at, answered_at, ended_at, updated_at
`

func (s *Store) SaveCall(ctx context.Context, value call.Call) (call.Call, error) {
	if err := s.requireStore(); err != nil {
		return call.Call{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Call{}, err
	}
	if s.tx != nil {
		return s.saveCall(ctx, value)
	}

	var saved call.Call
	err := s.WithinTx(ctx, func(tx call.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveCall(ctx, value)
		return saveErr
	})
	if err != nil {
		return call.Call{}, err
	}
	return saved, nil
}

func (s *Store) saveCall(ctx context.Context, value call.Call) (call.Call, error) {
	value.ID = strings.TrimSpace(value.ID)
	value.ConversationID = strings.TrimSpace(value.ConversationID)
	value.InitiatorAccountID = strings.TrimSpace(value.InitiatorAccountID)
	value.ActiveSessionID = strings.TrimSpace(value.ActiveSessionID)
	if value.ID == "" || value.ConversationID == "" || value.InitiatorAccountID == "" || value.State == call.StateUnspecified {
		return call.Call{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	call_id, conversation_id, initiator_account_id, active_session_id, requested_video,
	state, end_reason, started_at, answered_at, ended_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5,
	$6, $7, $8, $9, $10, $11
)
ON CONFLICT (call_id) DO UPDATE SET
	conversation_id = EXCLUDED.conversation_id,
	initiator_account_id = EXCLUDED.initiator_account_id,
	active_session_id = EXCLUDED.active_session_id,
	requested_video = EXCLUDED.requested_video,
	state = EXCLUDED.state,
	end_reason = EXCLUDED.end_reason,
	started_at = EXCLUDED.started_at,
	answered_at = EXCLUDED.answered_at,
	ended_at = EXCLUDED.ended_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("call_calls"), callColumnList)

	row := s.conn().QueryRowContext(
		ctx,
		query,
		value.ID,
		value.ConversationID,
		value.InitiatorAccountID,
		nullString(value.ActiveSessionID),
		value.RequestedVideo,
		value.State,
		nullString(string(value.EndReason)),
		value.StartedAt.UTC(),
		nullTime(value.AnsweredAt),
		nullTime(value.EndedAt),
		value.UpdatedAt.UTC(),
	)
	saved, err := scanCall(row)
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return call.Call{}, mapped
		}
		return call.Call{}, fmt.Errorf("save call %s: %w", value.ID, err)
	}
	return saved, nil
}

func (s *Store) CallByID(ctx context.Context, callID string) (call.Call, error) {
	if err := s.requireStore(); err != nil {
		return call.Call{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Call{}, err
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return call.Call{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE call_id = $1`, callColumnList, s.table("call_calls"))
	row := s.conn().QueryRowContext(ctx, query, callID)
	value, err := scanCall(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return call.Call{}, call.ErrNotFound
		}
		return call.Call{}, fmt.Errorf("load call %s: %w", callID, err)
	}
	return value, nil
}

func (s *Store) ActiveCallByConversation(ctx context.Context, conversationID string) (call.Call, error) {
	if err := s.requireStore(); err != nil {
		return call.Call{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Call{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return call.Call{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE conversation_id = $1 AND state IN ('ringing', 'active')
ORDER BY started_at DESC
LIMIT 1
`, callColumnList, s.table("call_calls"))
	row := s.conn().QueryRowContext(ctx, query, conversationID)
	value, err := scanCall(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return call.Call{}, call.ErrNotFound
		}
		return call.Call{}, fmt.Errorf("load active call for conversation %s: %w", conversationID, err)
	}
	return value, nil
}

func (s *Store) CallsByConversation(ctx context.Context, conversationID string, includeEnded bool) ([]call.Call, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE conversation_id = $1
`, callColumnList, s.table("call_calls"))
	if !includeEnded {
		query += ` AND state <> 'ended'`
	}
	query += ` ORDER BY started_at DESC, call_id ASC`

	rows, err := s.conn().QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list calls for conversation %s: %w", conversationID, err)
	}
	defer rows.Close()

	result := make([]call.Call, 0)
	for rows.Next() {
		value, scanErr := scanCall(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan call for conversation %s: %w", conversationID, scanErr)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate calls for conversation %s: %w", conversationID, err)
	}
	return result, nil
}

func nullString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
