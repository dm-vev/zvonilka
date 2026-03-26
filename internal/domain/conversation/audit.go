package conversation

import (
	"context"
	"fmt"
	"strings"
)

// LogModerationAction persists a moderation audit entry.
func (s *Service) LogModerationAction(
	ctx context.Context,
	action ModerationAction,
) (ModerationAction, error) {
	if err := s.validateContext(ctx, "log moderation action"); err != nil {
		return ModerationAction{}, err
	}

	action.TargetID = strings.TrimSpace(action.TargetID)
	action.ActorAccountID = strings.TrimSpace(action.ActorAccountID)
	action.TargetAccountID = strings.TrimSpace(action.TargetAccountID)
	action.Reason = strings.TrimSpace(action.Reason)
	if action.TargetKind == ModerationTargetKindUnspecified || action.TargetID == "" || action.ActorAccountID == "" || action.Type == ModerationActionTypeUnspecified {
		return ModerationAction{}, ErrInvalidInput
	}

	if action.ID == "" {
		actionID, err := newID("mod")
		if err != nil {
			return ModerationAction{}, fmt.Errorf("generate moderation action id: %w", err)
		}
		action.ID = actionID
	}
	if action.CreatedAt.IsZero() {
		action.CreatedAt = s.currentTime()
	}

	saved, err := s.store.SaveModerationAction(ctx, action)
	if err != nil {
		return ModerationAction{}, fmt.Errorf("save moderation action %s: %w", action.ID, err)
	}

	return saved, nil
}

// ModerationActionsByTarget lists the audit trail for a moderation target.
func (s *Service) ModerationActionsByTarget(
	ctx context.Context,
	targetKind ModerationTargetKind,
	targetID string,
) ([]ModerationAction, error) {
	if err := s.validateContext(ctx, "list moderation actions"); err != nil {
		return nil, err
	}

	targetID = strings.TrimSpace(targetID)
	if targetKind == ModerationTargetKindUnspecified || targetID == "" {
		return nil, ErrInvalidInput
	}

	actions, err := s.store.ModerationActionsByTarget(ctx, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("list moderation actions for %s:%s: %w", targetKind, targetID, err)
	}

	return actions, nil
}
