package pgstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/call"
)

const inviteColumnList = `
call_id, account_id, state, expires_at, answered_at, updated_at
`

func (s *Store) SaveInvite(ctx context.Context, value call.Invite) (call.Invite, error) {
	if err := s.requireStore(); err != nil {
		return call.Invite{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Invite{}, err
	}
	if s.tx != nil {
		return s.saveInvite(ctx, value)
	}

	var saved call.Invite
	err := s.WithinTx(ctx, func(tx call.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveInvite(ctx, value)
		return saveErr
	})
	if err != nil {
		return call.Invite{}, err
	}
	return saved, nil
}

func (s *Store) saveInvite(ctx context.Context, value call.Invite) (call.Invite, error) {
	value.CallID = strings.TrimSpace(value.CallID)
	value.AccountID = strings.TrimSpace(value.AccountID)
	if value.CallID == "" || value.AccountID == "" || value.State == call.InviteStateUnspecified || value.ExpiresAt.IsZero() || value.UpdatedAt.IsZero() {
		return call.Invite{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	call_id, account_id, state, expires_at, answered_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6
)
ON CONFLICT (call_id, account_id) DO UPDATE SET
	state = EXCLUDED.state,
	expires_at = EXCLUDED.expires_at,
	answered_at = EXCLUDED.answered_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("call_invites"), inviteColumnList)
	row := s.conn().QueryRowContext(
		ctx,
		query,
		value.CallID,
		value.AccountID,
		value.State,
		value.ExpiresAt.UTC(),
		nullTime(value.AnsweredAt),
		value.UpdatedAt.UTC(),
	)
	saved, err := scanInvite(row)
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return call.Invite{}, mapped
		}
		return call.Invite{}, fmt.Errorf("save invite %s/%s: %w", value.CallID, value.AccountID, err)
	}
	return saved, nil
}

func (s *Store) InviteByCallAndAccount(ctx context.Context, callID string, accountID string) (call.Invite, error) {
	if err := s.requireStore(); err != nil {
		return call.Invite{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return call.Invite{}, err
	}
	callID = strings.TrimSpace(callID)
	accountID = strings.TrimSpace(accountID)
	if callID == "" || accountID == "" {
		return call.Invite{}, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE call_id = $1 AND account_id = $2`, inviteColumnList, s.table("call_invites"))
	row := s.conn().QueryRowContext(ctx, query, callID, accountID)
	value, err := scanInvite(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return call.Invite{}, call.ErrNotFound
		}
		return call.Invite{}, fmt.Errorf("load invite %s/%s: %w", callID, accountID, err)
	}
	return value, nil
}

func (s *Store) InvitesByCall(ctx context.Context, callID string) ([]call.Invite, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, call.ErrInvalidInput
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE call_id = $1 ORDER BY account_id ASC`, inviteColumnList, s.table("call_invites"))
	rows, err := s.conn().QueryContext(ctx, query, callID)
	if err != nil {
		return nil, fmt.Errorf("list invites for call %s: %w", callID, err)
	}
	defer rows.Close()

	result := make([]call.Invite, 0)
	for rows.Next() {
		value, scanErr := scanInvite(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan invite for call %s: %w", callID, scanErr)
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invites for call %s: %w", callID, err)
	}
	return result, nil
}
