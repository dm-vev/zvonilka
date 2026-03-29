package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SaveModerationPolicy inserts or updates a moderation policy row.
func (s *Store) SaveModerationPolicy(ctx context.Context, policy conversation.ModerationPolicy) (conversation.ModerationPolicy, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationPolicy{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ModerationPolicy{}, err
	}
	if policy.TargetID == "" {
		return conversation.ModerationPolicy{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveModerationPolicy(ctx, policy)
	}

	var saved conversation.ModerationPolicy
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveModerationPolicy(ctx, policy)
		return saveErr
	})
	if err != nil {
		return conversation.ModerationPolicy{}, err
	}

	return saved, nil
}

func (s *Store) saveModerationPolicy(ctx context.Context, policy conversation.ModerationPolicy) (conversation.ModerationPolicy, error) {
	policy.TargetKind = conversation.ModerationTargetKind(normalizeName(string(policy.TargetKind)))
	policy.TargetID = strings.TrimSpace(policy.TargetID)
	if policy.TargetKind == conversation.ModerationTargetKindUnspecified || policy.TargetID == "" {
		return conversation.ModerationPolicy{}, conversation.ErrInvalidInput
	}
	if policy.CreatedAt.IsZero() || policy.UpdatedAt.IsZero() || policy.UpdatedAt.Before(policy.CreatedAt) {
		return conversation.ModerationPolicy{}, conversation.ErrInvalidInput
	}
	if (policy.AntiSpamWindow == 0) != (policy.AntiSpamBurstLimit == 0) {
		return conversation.ModerationPolicy{}, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	target_kind, target_id, only_admins_can_write, only_admins_can_add_members, allow_reactions,
	allow_forwards, allow_threads, require_encrypted_messages, require_trusted_devices, require_join_approval, pinned_messages_only_admins,
	slow_mode_interval_nanos, anti_spam_window_nanos, anti_spam_burst_limit, shadow_mode, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5,
	$6, $7, $8, $9, $10,
	$11, $12, $13, $14, $15, $16
)
ON CONFLICT (target_kind, target_id) DO UPDATE SET
	only_admins_can_write = EXCLUDED.only_admins_can_write,
	only_admins_can_add_members = EXCLUDED.only_admins_can_add_members,
	allow_reactions = EXCLUDED.allow_reactions,
	allow_forwards = EXCLUDED.allow_forwards,
	allow_threads = EXCLUDED.allow_threads,
	require_encrypted_messages = EXCLUDED.require_encrypted_messages,
	require_trusted_devices = EXCLUDED.require_trusted_devices,
	require_join_approval = EXCLUDED.require_join_approval,
	pinned_messages_only_admins = EXCLUDED.pinned_messages_only_admins,
	slow_mode_interval_nanos = EXCLUDED.slow_mode_interval_nanos,
	anti_spam_window_nanos = EXCLUDED.anti_spam_window_nanos,
	anti_spam_burst_limit = EXCLUDED.anti_spam_burst_limit,
	shadow_mode = EXCLUDED.shadow_mode,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("conversation_moderation_policies"), moderationPolicyColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		policy.TargetKind,
		policy.TargetID,
		policy.OnlyAdminsCanWrite,
		policy.OnlyAdminsCanAddMembers,
		policy.AllowReactions,
		policy.AllowForwards,
		policy.AllowThreads,
		policy.RequireEncryptedMessages,
		policy.RequireTrustedDevices,
		policy.RequireJoinApproval,
		policy.PinnedMessagesOnlyAdmins,
		int64(policy.SlowModeInterval),
		int64(policy.AntiSpamWindow),
		policy.AntiSpamBurstLimit,
		policy.ShadowMode,
		policy.CreatedAt.UTC(),
		policy.UpdatedAt.UTC(),
	)

	saved, err := scanModerationPolicy(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.ModerationPolicy{}, mappedErr
		}
		return conversation.ModerationPolicy{}, fmt.Errorf("save moderation policy %s:%s: %w", policy.TargetKind, policy.TargetID, err)
	}

	return saved, nil
}

// ModerationPolicyByTarget resolves a moderation policy row.
func (s *Store) ModerationPolicyByTarget(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string) (conversation.ModerationPolicy, error) {
	if err := s.requireStore(); err != nil {
		return conversation.ModerationPolicy{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationPolicy{}, err
	}
	targetKind = conversation.ModerationTargetKind(normalizeName(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" {
		return conversation.ModerationPolicy{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE target_kind = $1 AND target_id = $2`, moderationPolicyColumnList, s.table("conversation_moderation_policies"))
	row := s.conn().QueryRowContext(ctx, query, targetKind, targetID)
	policy, err := scanModerationPolicy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ModerationPolicy{}, conversation.ErrNotFound
		}
		return conversation.ModerationPolicy{}, fmt.Errorf("load moderation policy %s:%s: %w", targetKind, targetID, err)
	}

	return policy, nil
}

// SaveModerationReport inserts or updates a moderation report row.
func (s *Store) SaveModerationReport(ctx context.Context, report conversation.ModerationReport) (conversation.ModerationReport, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationReport{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ModerationReport{}, err
	}
	if report.ID == "" {
		return conversation.ModerationReport{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveModerationReport(ctx, report)
	}

	var saved conversation.ModerationReport
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveModerationReport(ctx, report)
		return saveErr
	})
	if err != nil {
		return conversation.ModerationReport{}, err
	}

	return saved, nil
}

func (s *Store) saveModerationReport(ctx context.Context, report conversation.ModerationReport) (conversation.ModerationReport, error) {
	report.ID = strings.TrimSpace(report.ID)
	report.TargetKind = conversation.ModerationTargetKind(normalizeName(string(report.TargetKind)))
	report.TargetID = strings.TrimSpace(report.TargetID)
	report.ReporterAccountID = strings.TrimSpace(report.ReporterAccountID)
	report.TargetAccountID = strings.TrimSpace(report.TargetAccountID)
	report.Reason = strings.TrimSpace(report.Reason)
	report.Details = strings.TrimSpace(report.Details)
	report.Resolution = strings.TrimSpace(report.Resolution)
	report.ReviewedByAccountID = strings.TrimSpace(report.ReviewedByAccountID)
	if report.ID == "" || report.TargetKind == conversation.ModerationTargetKindUnspecified || report.TargetID == "" || report.ReporterAccountID == "" || report.Reason == "" || report.Status == conversation.ModerationReportStatusUnspecified {
		return conversation.ModerationReport{}, conversation.ErrInvalidInput
	}
	if report.CreatedAt.IsZero() || report.UpdatedAt.IsZero() || report.UpdatedAt.Before(report.CreatedAt) {
		return conversation.ModerationReport{}, conversation.ErrInvalidInput
	}
	switch report.Status {
	case conversation.ModerationReportStatusPending:
		if report.ReviewedByAccountID != "" || !report.ReviewedAt.IsZero() {
			return conversation.ModerationReport{}, conversation.ErrInvalidInput
		}
	case conversation.ModerationReportStatusResolved, conversation.ModerationReportStatusRejected:
		if report.ReviewedByAccountID == "" || report.ReviewedAt.IsZero() {
			return conversation.ModerationReport{}, conversation.ErrInvalidInput
		}
	default:
		return conversation.ModerationReport{}, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, target_kind, target_id, reporter_account_id, target_account_id, reason, details, status,
	reviewed_by_account_id, reviewed_at, resolution, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8,
	$9, $10, $11, $12, $13
)
ON CONFLICT (id) DO UPDATE SET
	target_kind = EXCLUDED.target_kind,
	target_id = EXCLUDED.target_id,
	reporter_account_id = EXCLUDED.reporter_account_id,
	target_account_id = EXCLUDED.target_account_id,
	reason = EXCLUDED.reason,
	details = EXCLUDED.details,
	status = EXCLUDED.status,
	reviewed_by_account_id = EXCLUDED.reviewed_by_account_id,
	reviewed_at = EXCLUDED.reviewed_at,
	resolution = EXCLUDED.resolution,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("conversation_moderation_reports"), moderationReportColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		report.ID,
		report.TargetKind,
		report.TargetID,
		report.ReporterAccountID,
		report.TargetAccountID,
		report.Reason,
		report.Details,
		report.Status,
		report.ReviewedByAccountID,
		encodeTime(report.ReviewedAt),
		report.Resolution,
		report.CreatedAt.UTC(),
		report.UpdatedAt.UTC(),
	)

	saved, err := scanModerationReport(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.ModerationReport{}, mappedErr
		}
		return conversation.ModerationReport{}, fmt.Errorf("save moderation report %s: %w", report.ID, err)
	}

	return saved, nil
}

// ModerationReportByID resolves a moderation report by identifier.
func (s *Store) ModerationReportByID(ctx context.Context, reportID string) (conversation.ModerationReport, error) {
	if err := s.requireStore(); err != nil {
		return conversation.ModerationReport{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationReport{}, err
	}
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		return conversation.ModerationReport{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, moderationReportColumnList, s.table("conversation_moderation_reports"))
	row := s.conn().QueryRowContext(ctx, query, reportID)
	report, err := scanModerationReport(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ModerationReport{}, conversation.ErrNotFound
		}
		return conversation.ModerationReport{}, fmt.Errorf("load moderation report %s: %w", reportID, err)
	}

	return report, nil
}

// ModerationReportsByTarget lists moderation reports for a target.
func (s *Store) ModerationReportsByTarget(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string) ([]conversation.ModerationReport, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	targetKind = conversation.ModerationTargetKind(normalizeName(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" {
		return nil, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE target_kind = $1 AND target_id = $2
ORDER BY created_at DESC, id ASC
`, moderationReportColumnList, s.table("conversation_moderation_reports"))
	rows, err := s.conn().QueryContext(ctx, query, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("list moderation reports for %s:%s: %w", targetKind, targetID, err)
	}
	defer rows.Close()

	reports := make([]conversation.ModerationReport, 0)
	for rows.Next() {
		report, scanErr := scanModerationReport(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan moderation report for %s:%s: %w", targetKind, targetID, scanErr)
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate moderation reports for %s:%s: %w", targetKind, targetID, err)
	}

	return reports, nil
}

// SaveModerationAction inserts a moderation audit entry.
func (s *Store) SaveModerationAction(ctx context.Context, action conversation.ModerationAction) (conversation.ModerationAction, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationAction{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ModerationAction{}, err
	}
	if action.ID == "" {
		return conversation.ModerationAction{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveModerationAction(ctx, action)
	}

	var saved conversation.ModerationAction
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveModerationAction(ctx, action)
		return saveErr
	})
	if err != nil {
		return conversation.ModerationAction{}, err
	}

	return saved, nil
}

func (s *Store) saveModerationAction(ctx context.Context, action conversation.ModerationAction) (conversation.ModerationAction, error) {
	action.ID = strings.TrimSpace(action.ID)
	action.TargetKind = conversation.ModerationTargetKind(normalizeName(string(action.TargetKind)))
	action.TargetID = strings.TrimSpace(action.TargetID)
	action.ActorAccountID = strings.TrimSpace(action.ActorAccountID)
	action.TargetAccountID = strings.TrimSpace(action.TargetAccountID)
	action.Reason = strings.TrimSpace(action.Reason)
	if action.ID == "" || action.TargetKind == conversation.ModerationTargetKindUnspecified || action.TargetID == "" || action.ActorAccountID == "" || action.Type == conversation.ModerationActionTypeUnspecified {
		return conversation.ModerationAction{}, conversation.ErrInvalidInput
	}
	if action.CreatedAt.IsZero() {
		return conversation.ModerationAction{}, conversation.ErrInvalidInput
	}

	metadata, err := encodeMetadata(action.Metadata)
	if err != nil {
		return conversation.ModerationAction{}, fmt.Errorf("encode moderation action metadata: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	id, target_kind, target_id, actor_account_id, target_account_id, type, duration_seconds, reason, metadata, created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING %s
`, s.table("conversation_moderation_actions"), moderationActionColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		action.ID,
		action.TargetKind,
		action.TargetID,
		action.ActorAccountID,
		action.TargetAccountID,
		action.Type,
		int64(action.Duration/time.Second),
		action.Reason,
		metadata,
		action.CreatedAt.UTC(),
	)

	saved, err := scanModerationAction(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.ModerationAction{}, mappedErr
		}
		return conversation.ModerationAction{}, fmt.Errorf("save moderation action %s: %w", action.ID, err)
	}

	return saved, nil
}

// ModerationActionsByTarget lists moderation audit rows for a target.
func (s *Store) ModerationActionsByTarget(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string) ([]conversation.ModerationAction, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	targetKind = conversation.ModerationTargetKind(normalizeName(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" {
		return nil, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE target_kind = $1 AND target_id = $2
ORDER BY created_at DESC, id ASC
`, moderationActionColumnList, s.table("conversation_moderation_actions"))
	rows, err := s.conn().QueryContext(ctx, query, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("list moderation actions for %s:%s: %w", targetKind, targetID, err)
	}
	defer rows.Close()

	actions := make([]conversation.ModerationAction, 0)
	for rows.Next() {
		action, scanErr := scanModerationAction(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan moderation action for %s:%s: %w", targetKind, targetID, scanErr)
		}
		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate moderation actions for %s:%s: %w", targetKind, targetID, err)
	}

	return actions, nil
}

// SaveModerationRestriction inserts or updates a moderation restriction.
func (s *Store) SaveModerationRestriction(ctx context.Context, restriction conversation.ModerationRestriction) (conversation.ModerationRestriction, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationRestriction{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ModerationRestriction{}, err
	}
	if restriction.TargetID == "" || restriction.AccountID == "" {
		return conversation.ModerationRestriction{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveModerationRestriction(ctx, restriction)
	}

	var saved conversation.ModerationRestriction
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveModerationRestriction(ctx, restriction)
		return saveErr
	})
	if err != nil {
		return conversation.ModerationRestriction{}, err
	}

	return saved, nil
}

func (s *Store) saveModerationRestriction(ctx context.Context, restriction conversation.ModerationRestriction) (conversation.ModerationRestriction, error) {
	restriction.TargetKind = conversation.ModerationTargetKind(normalizeName(string(restriction.TargetKind)))
	restriction.TargetID = strings.TrimSpace(restriction.TargetID)
	restriction.AccountID = strings.TrimSpace(restriction.AccountID)
	restriction.AppliedByAccountID = strings.TrimSpace(restriction.AppliedByAccountID)
	restriction.Reason = strings.TrimSpace(restriction.Reason)
	if restriction.TargetKind == conversation.ModerationTargetKindUnspecified || restriction.TargetID == "" || restriction.AccountID == "" || restriction.State == conversation.ModerationRestrictionStateUnspecified {
		return conversation.ModerationRestriction{}, conversation.ErrInvalidInput
	}
	if restriction.CreatedAt.IsZero() || restriction.UpdatedAt.IsZero() || restriction.UpdatedAt.Before(restriction.CreatedAt) {
		return conversation.ModerationRestriction{}, conversation.ErrInvalidInput
	}
	if !restriction.ExpiresAt.IsZero() && restriction.ExpiresAt.Before(restriction.CreatedAt) {
		return conversation.ModerationRestriction{}, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	target_kind, target_id, account_id, state, applied_by_account_id, reason, expires_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
)
ON CONFLICT (target_kind, target_id, account_id) DO UPDATE SET
	state = EXCLUDED.state,
	applied_by_account_id = EXCLUDED.applied_by_account_id,
	reason = EXCLUDED.reason,
	expires_at = EXCLUDED.expires_at,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("conversation_moderation_restrictions"), moderationRestrictionColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		restriction.TargetKind,
		restriction.TargetID,
		restriction.AccountID,
		restriction.State,
		restriction.AppliedByAccountID,
		restriction.Reason,
		encodeTime(restriction.ExpiresAt),
		restriction.CreatedAt.UTC(),
		restriction.UpdatedAt.UTC(),
	)

	saved, err := scanModerationRestriction(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.ModerationRestriction{}, mappedErr
		}
		return conversation.ModerationRestriction{}, fmt.Errorf("save moderation restriction for %s:%s and account %s: %w", restriction.TargetKind, restriction.TargetID, restriction.AccountID, err)
	}

	return saved, nil
}

// ModerationRestrictionByTargetAndAccount resolves one moderation restriction.
func (s *Store) ModerationRestrictionByTargetAndAccount(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string, accountID string) (conversation.ModerationRestriction, error) {
	if err := s.requireStore(); err != nil {
		return conversation.ModerationRestriction{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationRestriction{}, err
	}
	targetKind = conversation.ModerationTargetKind(normalizeName(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	accountID = strings.TrimSpace(accountID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" || accountID == "" {
		return conversation.ModerationRestriction{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE target_kind = $1 AND target_id = $2 AND account_id = $3`, moderationRestrictionColumnList, s.table("conversation_moderation_restrictions"))
	row := s.conn().QueryRowContext(ctx, query, targetKind, targetID, accountID)
	restriction, err := scanModerationRestriction(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ModerationRestriction{}, conversation.ErrNotFound
		}
		return conversation.ModerationRestriction{}, fmt.Errorf("load moderation restriction for %s:%s and account %s: %w", targetKind, targetID, accountID, err)
	}

	return restriction, nil
}

// ModerationRestrictionsByTarget lists restrictions for a target.
func (s *Store) ModerationRestrictionsByTarget(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string) ([]conversation.ModerationRestriction, error) {
	if err := s.requireStore(); err != nil {
		return nil, err
	}
	if err := s.requireContext(ctx); err != nil {
		return nil, err
	}
	targetKind = conversation.ModerationTargetKind(normalizeName(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" {
		return nil, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT %s
FROM %s
WHERE target_kind = $1 AND target_id = $2
ORDER BY updated_at DESC, account_id ASC
`, moderationRestrictionColumnList, s.table("conversation_moderation_restrictions"))
	rows, err := s.conn().QueryContext(ctx, query, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("list moderation restrictions for %s:%s: %w", targetKind, targetID, err)
	}
	defer rows.Close()

	restrictions := make([]conversation.ModerationRestriction, 0)
	for rows.Next() {
		restriction, scanErr := scanModerationRestriction(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan moderation restriction for %s:%s: %w", targetKind, targetID, scanErr)
		}
		restrictions = append(restrictions, restriction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate moderation restrictions for %s:%s: %w", targetKind, targetID, err)
	}

	return restrictions, nil
}

// DeleteModerationRestriction removes a moderation restriction row.
func (s *Store) DeleteModerationRestriction(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string, accountID string) error {
	if err := s.requireContext(ctx); err != nil {
		return err
	}
	if err := s.requireStore(); err != nil {
		return err
	}
	if s.tx != nil {
		return s.deleteModerationRestriction(ctx, targetKind, targetID, accountID)
	}

	return s.withTransaction(ctx, func(tx conversation.Store) error {
		return tx.(*Store).deleteModerationRestriction(ctx, targetKind, targetID, accountID)
	})
}

func (s *Store) deleteModerationRestriction(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string, accountID string) error {
	targetKind = conversation.ModerationTargetKind(normalizeName(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	accountID = strings.TrimSpace(accountID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" || accountID == "" {
		return conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE target_kind = $1 AND target_id = $2 AND account_id = $3`, s.table("conversation_moderation_restrictions"))
	if _, err := s.conn().ExecContext(ctx, query, targetKind, targetID, accountID); err != nil {
		return fmt.Errorf("delete moderation restriction for %s:%s and account %s: %w", targetKind, targetID, accountID, err)
	}

	return nil
}

// SaveModerationRateState inserts or updates a moderation rate state row.
func (s *Store) SaveModerationRateState(ctx context.Context, state conversation.ModerationRateState) (conversation.ModerationRateState, error) {
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationRateState{}, err
	}
	if err := s.requireStore(); err != nil {
		return conversation.ModerationRateState{}, err
	}
	if state.TargetID == "" || state.AccountID == "" {
		return conversation.ModerationRateState{}, conversation.ErrInvalidInput
	}

	if s.tx != nil {
		return s.saveModerationRateState(ctx, state)
	}

	var saved conversation.ModerationRateState
	err := s.withTransaction(ctx, func(tx conversation.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).saveModerationRateState(ctx, state)
		return saveErr
	})
	if err != nil {
		return conversation.ModerationRateState{}, err
	}

	return saved, nil
}

func (s *Store) saveModerationRateState(ctx context.Context, state conversation.ModerationRateState) (conversation.ModerationRateState, error) {
	state.TargetKind = conversation.ModerationTargetKind(normalizeName(string(state.TargetKind)))
	state.TargetID = strings.TrimSpace(state.TargetID)
	state.AccountID = strings.TrimSpace(state.AccountID)
	if state.TargetKind == conversation.ModerationTargetKindUnspecified || state.TargetID == "" || state.AccountID == "" {
		return conversation.ModerationRateState{}, conversation.ErrInvalidInput
	}
	if state.UpdatedAt.IsZero() {
		return conversation.ModerationRateState{}, conversation.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	target_kind, target_id, account_id, last_write_at, window_started_at, window_count, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (target_kind, target_id, account_id) DO UPDATE SET
	last_write_at = EXCLUDED.last_write_at,
	window_started_at = EXCLUDED.window_started_at,
	window_count = EXCLUDED.window_count,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("conversation_moderation_rate_states"), moderationRateStateColumnList)

	row := s.conn().QueryRowContext(ctx, query,
		state.TargetKind,
		state.TargetID,
		state.AccountID,
		encodeTime(state.LastWriteAt),
		encodeTime(state.WindowStartedAt),
		state.WindowCount,
		state.UpdatedAt.UTC(),
	)

	saved, err := scanModerationRateState(row)
	if err != nil {
		if mappedErr := mapConstraintError(err, conversation.ErrNotFound); mappedErr != nil {
			return conversation.ModerationRateState{}, mappedErr
		}
		return conversation.ModerationRateState{}, fmt.Errorf("save moderation rate state for %s:%s and account %s: %w", state.TargetKind, state.TargetID, state.AccountID, err)
	}

	return saved, nil
}

// ModerationRateStateByTargetAndAccount resolves one moderation rate state row.
func (s *Store) ModerationRateStateByTargetAndAccount(ctx context.Context, targetKind conversation.ModerationTargetKind, targetID string, accountID string) (conversation.ModerationRateState, error) {
	if err := s.requireStore(); err != nil {
		return conversation.ModerationRateState{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return conversation.ModerationRateState{}, err
	}
	targetKind = conversation.ModerationTargetKind(normalizeName(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	accountID = strings.TrimSpace(accountID)
	if targetKind == conversation.ModerationTargetKindUnspecified || targetID == "" || accountID == "" {
		return conversation.ModerationRateState{}, conversation.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE target_kind = $1 AND target_id = $2 AND account_id = $3`, moderationRateStateColumnList, s.table("conversation_moderation_rate_states"))
	row := s.conn().QueryRowContext(ctx, query, targetKind, targetID, accountID)
	state, err := scanModerationRateState(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ModerationRateState{}, conversation.ErrNotFound
		}
		return conversation.ModerationRateState{}, fmt.Errorf("load moderation rate state for %s:%s and account %s: %w", targetKind, targetID, accountID, err)
	}

	return state, nil
}

func scanModerationPolicy(row rowScanner) (conversation.ModerationPolicy, error) {
	var (
		policy                conversation.ModerationPolicy
		slowModeIntervalNanos int64
		antiSpamWindowNanos   int64
	)

	if err := row.Scan(
		&policy.TargetKind,
		&policy.TargetID,
		&policy.OnlyAdminsCanWrite,
		&policy.OnlyAdminsCanAddMembers,
		&policy.AllowReactions,
		&policy.AllowForwards,
		&policy.AllowThreads,
		&policy.RequireEncryptedMessages,
		&policy.RequireTrustedDevices,
		&policy.RequireJoinApproval,
		&policy.PinnedMessagesOnlyAdmins,
		&slowModeIntervalNanos,
		&antiSpamWindowNanos,
		&policy.AntiSpamBurstLimit,
		&policy.ShadowMode,
		&policy.CreatedAt,
		&policy.UpdatedAt,
	); err != nil {
		return conversation.ModerationPolicy{}, err
	}

	policy.SlowModeInterval = time.Duration(slowModeIntervalNanos)
	policy.AntiSpamWindow = time.Duration(antiSpamWindowNanos)
	policy.CreatedAt = policy.CreatedAt.UTC()
	policy.UpdatedAt = policy.UpdatedAt.UTC()
	return policy, nil
}

func scanModerationReport(row rowScanner) (conversation.ModerationReport, error) {
	var (
		report     conversation.ModerationReport
		reviewedAt sql.NullTime
	)

	if err := row.Scan(
		&report.ID,
		&report.TargetKind,
		&report.TargetID,
		&report.ReporterAccountID,
		&report.TargetAccountID,
		&report.Reason,
		&report.Details,
		&report.Status,
		&report.ReviewedByAccountID,
		&reviewedAt,
		&report.Resolution,
		&report.CreatedAt,
		&report.UpdatedAt,
	); err != nil {
		return conversation.ModerationReport{}, err
	}

	report.CreatedAt = report.CreatedAt.UTC()
	report.UpdatedAt = report.UpdatedAt.UTC()
	report.ReviewedAt = decodeTime(reviewedAt)
	return report, nil
}

func scanModerationAction(row rowScanner) (conversation.ModerationAction, error) {
	var (
		action          conversation.ModerationAction
		metadata        string
		durationSeconds int64
	)

	if err := row.Scan(
		&action.ID,
		&action.TargetKind,
		&action.TargetID,
		&action.ActorAccountID,
		&action.TargetAccountID,
		&action.Type,
		&durationSeconds,
		&action.Reason,
		&metadata,
		&action.CreatedAt,
	); err != nil {
		return conversation.ModerationAction{}, err
	}

	action.Duration = time.Duration(durationSeconds) * time.Second
	action.CreatedAt = action.CreatedAt.UTC()
	if decodedMetadata, err := decodeMetadata(metadata); err == nil {
		action.Metadata = decodedMetadata
	} else {
		return conversation.ModerationAction{}, err
	}

	return action, nil
}

func scanModerationRestriction(row rowScanner) (conversation.ModerationRestriction, error) {
	var (
		restriction conversation.ModerationRestriction
		expiresAt   sql.NullTime
	)

	if err := row.Scan(
		&restriction.TargetKind,
		&restriction.TargetID,
		&restriction.AccountID,
		&restriction.State,
		&restriction.AppliedByAccountID,
		&restriction.Reason,
		&expiresAt,
		&restriction.CreatedAt,
		&restriction.UpdatedAt,
	); err != nil {
		return conversation.ModerationRestriction{}, err
	}

	restriction.CreatedAt = restriction.CreatedAt.UTC()
	restriction.UpdatedAt = restriction.UpdatedAt.UTC()
	restriction.ExpiresAt = decodeTime(expiresAt)
	return restriction, nil
}

func scanModerationRateState(row rowScanner) (conversation.ModerationRateState, error) {
	var (
		state         conversation.ModerationRateState
		lastWriteAt   sql.NullTime
		windowStarted sql.NullTime
	)

	if err := row.Scan(
		&state.TargetKind,
		&state.TargetID,
		&state.AccountID,
		&lastWriteAt,
		&windowStarted,
		&state.WindowCount,
		&state.UpdatedAt,
	); err != nil {
		return conversation.ModerationRateState{}, err
	}

	state.LastWriteAt = decodeTime(lastWriteAt)
	state.WindowStartedAt = decodeTime(windowStarted)
	state.UpdatedAt = state.UpdatedAt.UTC()
	return state, nil
}
