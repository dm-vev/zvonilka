package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// SaveJoinRequest inserts or replaces a join request.
func (s *Store) SaveJoinRequest(ctx context.Context, joinRequest identity.JoinRequest) (identity.JoinRequest, error) {
	if joinRequest.ID == "" {
		return identity.JoinRequest{}, identity.ErrInvalidInput
	}
	if s.tx != nil {
		return s.saveJoinRequest(ctx, joinRequest)
	}

	var saved identity.JoinRequest
	err := s.withTransaction(ctx, func(tx identity.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveJoinRequest(ctx, joinRequest)
		return saveErr
	})
	if err != nil {
		return identity.JoinRequest{}, err
	}

	return saved, nil
}

func (s *Store) saveJoinRequest(ctx context.Context, joinRequest identity.JoinRequest) (identity.JoinRequest, error) {
	joinRequest.ID = strings.TrimSpace(joinRequest.ID)
	joinRequest.Username = strings.TrimSpace(joinRequest.Username)
	joinRequest.DisplayName = strings.TrimSpace(joinRequest.DisplayName)
	joinRequest.Email = strings.TrimSpace(joinRequest.Email)
	joinRequest.Phone = strings.TrimSpace(joinRequest.Phone)
	joinRequest.Note = strings.TrimSpace(joinRequest.Note)
	joinRequest.ReviewedBy = strings.TrimSpace(joinRequest.ReviewedBy)
	joinRequest.DecisionReason = strings.TrimSpace(joinRequest.DecisionReason)

	if joinRequest.Status == identity.JoinRequestStatusPending && joinRequest.ReviewedAt.IsZero() {
		if err := s.lockIdentifiers(ctx, s.joinLockKeys(joinRequest)...); err != nil {
			return identity.JoinRequest{}, err
		}

		if conflict, err := s.accountConflictForJoin(ctx, joinRequest); err != nil {
			return identity.JoinRequest{}, err
		} else if conflict {
			return identity.JoinRequest{}, identity.ErrConflict
		}

		if err := s.expireStaleJoinRequests(ctx, joinRequest); err != nil {
			return identity.JoinRequest{}, err
		}
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, username, display_name, email, phone, note, status, requested_at, reviewed_at, reviewed_by, decision_reason, expires_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT (id) DO UPDATE SET
	username = EXCLUDED.username,
	display_name = EXCLUDED.display_name,
	email = EXCLUDED.email,
	phone = EXCLUDED.phone,
	note = EXCLUDED.note,
	status = EXCLUDED.status,
	reviewed_at = EXCLUDED.reviewed_at,
	reviewed_by = EXCLUDED.reviewed_by,
	decision_reason = EXCLUDED.decision_reason,
	expires_at = EXCLUDED.expires_at
RETURNING %s
`, s.table("identity_join_requests"), joinRequestColumnList)

	savedJoinRequest, err := scanJoinRequest(s.conn().QueryRowContext(ctx, query,
		joinRequest.ID,
		joinRequest.Username,
		joinRequest.DisplayName,
		joinRequest.Email,
		joinRequest.Phone,
		joinRequest.Note,
		joinRequest.Status,
		joinRequest.RequestedAt.UTC(),
		nullTime(joinRequest.ReviewedAt),
		joinRequest.ReviewedBy,
		joinRequest.DecisionReason,
		joinRequest.ExpiresAt.UTC(),
	))
	if err != nil {
		if isUniqueViolation(err) {
			return identity.JoinRequest{}, identity.ErrConflict
		}
		return identity.JoinRequest{}, fmt.Errorf("save join request %s: %w", joinRequest.ID, err)
	}

	return savedJoinRequest, nil
}

// JoinRequestByID resolves a join request by primary key.
func (s *Store) JoinRequestByID(ctx context.Context, joinRequestID string) (identity.JoinRequest, error) {
	if strings.TrimSpace(joinRequestID) == "" {
		return identity.JoinRequest{}, identity.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, joinRequestColumnList, s.table("identity_join_requests"))
	joinRequest, err := scanJoinRequest(s.conn().QueryRowContext(ctx, query, joinRequestID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.JoinRequest{}, identity.ErrNotFound
		}
		return identity.JoinRequest{}, fmt.Errorf("load join request %s: %w", joinRequestID, err)
	}

	return joinRequest, nil
}

// JoinRequestsByStatus lists join requests with the requested status.
func (s *Store) JoinRequestsByStatus(ctx context.Context, status identity.JoinRequestStatus) ([]identity.JoinRequest, error) {
	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE status = $1 ORDER BY requested_at ASC, id ASC`,
		joinRequestColumnList,
		s.table("identity_join_requests"),
	)
	rows, err := s.conn().QueryContext(ctx, query, status)
	if err != nil {
		return nil, fmt.Errorf("list join requests by status %s: %w", status, err)
	}
	defer rows.Close()

	joinRequests := make([]identity.JoinRequest, 0)
	for rows.Next() {
		joinRequest, scanErr := scanJoinRequest(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan join request by status %s: %w", status, scanErr)
		}
		joinRequests = append(joinRequests, joinRequest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate join requests by status %s: %w", status, err)
	}

	return joinRequests, nil
}

func (s *Store) accountConflictForJoin(ctx context.Context, joinRequest identity.JoinRequest) (bool, error) {
	if conflict, err := s.accountConflictByColumn(ctx, "username", joinRequest.Username, ""); err != nil || conflict {
		return conflict, err
	}
	if conflict, err := s.accountConflictByColumn(ctx, "email", joinRequest.Email, ""); err != nil || conflict {
		return conflict, err
	}
	if conflict, err := s.accountConflictByColumn(ctx, "phone", joinRequest.Phone, ""); err != nil || conflict {
		return conflict, err
	}

	return false, nil
}

func (s *Store) expireStaleJoinRequests(ctx context.Context, joinRequest identity.JoinRequest) error {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 4)
	args = append(args, identity.JoinRequestStatusPending)

	if strings.TrimSpace(joinRequest.Username) != "" {
		clauses = append(clauses, fmt.Sprintf("username = $%d", len(args)+1))
		args = append(args, joinRequest.Username)
	}
	if strings.TrimSpace(joinRequest.Email) != "" {
		clauses = append(clauses, fmt.Sprintf("email = $%d", len(args)+1))
		args = append(args, joinRequest.Email)
	}
	if strings.TrimSpace(joinRequest.Phone) != "" {
		clauses = append(clauses, fmt.Sprintf("phone = $%d", len(args)+1))
		args = append(args, joinRequest.Phone)
	}
	if len(clauses) == 0 {
		return nil
	}

	query := fmt.Sprintf(
		`SELECT %s FROM %s WHERE status = $1 AND (%s)`,
		joinRequestColumnList,
		s.table("identity_join_requests"),
		strings.Join(clauses, " OR "),
	)
	rows, err := s.conn().QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("load stale join requests for %s: %w", joinRequest.ID, err)
	}
	defer rows.Close()

	staleRequests := make(map[string]identity.JoinRequest)
	for rows.Next() {
		stale, scanErr := scanJoinRequest(rows)
		if scanErr != nil {
			return fmt.Errorf("scan stale join request for %s: %w", joinRequest.ID, scanErr)
		}
		staleRequests[stale.ID] = stale
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate stale join requests for %s: %w", joinRequest.ID, err)
	}

	now := joinRequest.RequestedAt
	for _, stale := range staleRequests {
		if now.Before(stale.ExpiresAt) {
			continue
		}

		stale.Status = identity.JoinRequestStatusExpired
		stale.ReviewedAt = now
		stale.ReviewedBy = ""
		stale.DecisionReason = "join request expired"

		updateQuery := fmt.Sprintf(`
UPDATE %s
SET status = $2, reviewed_at = $3, reviewed_by = $4, decision_reason = $5
WHERE id = $1
`, s.table("identity_join_requests"))

		if _, err := s.conn().ExecContext(
			ctx,
			updateQuery,
			stale.ID,
			stale.Status,
			nullTime(stale.ReviewedAt),
			stale.ReviewedBy,
			stale.DecisionReason,
		); err != nil {
			return fmt.Errorf("expire stale join request %s: %w", stale.ID, err)
		}
	}

	return nil
}
