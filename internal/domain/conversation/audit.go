package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
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
	if action.CreatedAt.IsZero() {
		action.CreatedAt = s.currentTime()
	}

	var saved ModerationAction
	err := s.runTx(ctx, func(tx Store) error {
		conversationID, resolveErr := moderationConversationID(action.TargetKind, action.TargetID)
		if resolveErr != nil {
			return resolveErr
		}

		var saveErr error
		saved, _, saveErr = s.saveModerationActionWithSyncEvent(ctx, tx, conversationID, action)
		return saveErr
	})
	if err != nil {
		return ModerationAction{}, err
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

func (s *Service) saveModerationActionWithSyncEvent(
	ctx context.Context,
	tx Store,
	conversationID string,
	action ModerationAction,
) (ModerationAction, EventEnvelope, error) {
	saved, err := saveModerationAction(ctx, tx, action)
	if err != nil {
		return ModerationAction{}, EventEnvelope{}, err
	}
	if !syncVisibleModerationAction(saved.Type) {
		return saved, EventEnvelope{}, nil
	}

	event, err := tx.SaveEvent(ctx, EventEnvelope{
		EventID:        eventID(),
		EventType:      EventTypeAdminActionRecorded,
		ConversationID: conversationID,
		ActorAccountID: saved.ActorAccountID,
		PayloadType:    "moderation_action",
		Metadata:       moderationEventMetadata(saved),
		CreatedAt:      saved.CreatedAt,
	})
	if err != nil {
		return ModerationAction{}, EventEnvelope{}, fmt.Errorf(
			"save moderation sync event for action %s: %w",
			saved.ID,
			err,
		)
	}

	return saved, event, nil
}

func saveModerationAction(
	ctx context.Context,
	tx Store,
	action ModerationAction,
) (ModerationAction, error) {
	action.TargetID = strings.TrimSpace(action.TargetID)
	action.ActorAccountID = strings.TrimSpace(action.ActorAccountID)
	action.TargetAccountID = strings.TrimSpace(action.TargetAccountID)
	action.Reason = strings.TrimSpace(action.Reason)
	action.Metadata = trimMetadata(action.Metadata)
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
		action.CreatedAt = time.Now().UTC()
	}

	saved, err := tx.SaveModerationAction(ctx, action)
	if err != nil {
		return ModerationAction{}, fmt.Errorf("save moderation action %s: %w", action.ID, err)
	}

	return saved, nil
}

func moderationConversationID(targetKind ModerationTargetKind, targetID string) (string, error) {
	targetID = strings.TrimSpace(targetID)
	switch targetKind {
	case ModerationTargetKindConversation, ModerationTargetKindChannel:
		if targetID == "" {
			return "", ErrInvalidInput
		}
		return targetID, nil
	case ModerationTargetKindTopic:
		conversationID, topicID := splitModerationKey(targetID)
		if conversationID == "" || topicID == "" {
			return "", ErrInvalidInput
		}
		return conversationID, nil
	default:
		return "", ErrInvalidInput
	}
}

func moderationEventMetadata(action ModerationAction) map[string]string {
	metadata := map[string]string{
		"action_id":   action.ID,
		"action_type": string(action.Type),
		"target_kind": string(action.TargetKind),
		"target_id":   action.TargetID,
	}
	if action.TargetAccountID != "" {
		metadata["target_account_id"] = action.TargetAccountID
	}
	if action.Duration > 0 {
		metadata["duration_nanos"] = fmt.Sprintf("%d", action.Duration)
	}

	return metadata
}

func syncVisibleModerationAction(actionType ModerationActionType) bool {
	switch actionType {
	case ModerationActionTypeBan,
		ModerationActionTypeKick,
		ModerationActionTypeMute,
		ModerationActionTypeUnmute,
		ModerationActionTypeShadowBan,
		ModerationActionTypeShadowUnban,
		ModerationActionTypePolicySet,
		ModerationActionTypeSlowModeSet,
		ModerationActionTypeAntiSpamSet:
		return true
	default:
		return false
	}
}
