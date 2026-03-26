package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ApplyModerationRestriction persists or updates a moderation restriction.
func (s *Service) ApplyModerationRestriction(
	ctx context.Context,
	params ApplyModerationRestrictionParams,
) (ModerationRestriction, error) {
	if err := s.validateContext(ctx, "apply moderation restriction"); err != nil {
		return ModerationRestriction{}, err
	}

	params.TargetID = strings.TrimSpace(params.TargetID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	params.Reason = strings.TrimSpace(params.Reason)
	if params.TargetKind == ModerationTargetKindUnspecified || params.TargetID == "" || params.ActorAccountID == "" || params.TargetAccountID == "" || params.State == ModerationRestrictionStateUnspecified {
		return ModerationRestriction{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var saved ModerationRestriction
	err := s.runTx(ctx, func(tx Store) error {
		conversation, err := s.lookupModerationConversation(ctx, tx, params.TargetKind, params.TargetID)
		if err != nil {
			return err
		}

		member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.ActorAccountID)
		if err != nil {
			return fmt.Errorf(
				"authorize restriction change for account %s in conversation %s: %w",
				params.ActorAccountID,
				conversation.ID,
				err,
			)
		}
		if !s.canModerate(member, params.ActorAccountID) {
			return ErrForbidden
		}

		restriction := ModerationRestriction{
			TargetKind:         params.TargetKind,
			TargetID:           params.TargetID,
			AccountID:          params.TargetAccountID,
			State:              params.State,
			AppliedByAccountID: params.ActorAccountID,
			Reason:             params.Reason,
			ExpiresAt:          timeOrZero(params.Duration, now),
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		saved, err = tx.SaveModerationRestriction(ctx, restriction)
		if err != nil {
			return fmt.Errorf(
				"save moderation restriction for %s in %s:%s: %w",
				params.TargetAccountID,
				params.TargetKind,
				params.TargetID,
				err,
			)
		}

		actionID, idErr := newID("mod")
		if idErr != nil {
			return fmt.Errorf("generate moderation action id: %w", idErr)
		}
		if _, err := tx.SaveModerationAction(ctx, ModerationAction{
			ID:              actionID,
			TargetKind:      params.TargetKind,
			TargetID:        params.TargetID,
			ActorAccountID:  params.ActorAccountID,
			TargetAccountID: params.TargetAccountID,
			Type:            restrictionActionType(params.State),
			Duration:        params.Duration,
			Reason:          params.Reason,
			Metadata: map[string]string{
				"state": string(params.State),
			},
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("log moderation restriction for %s:%s: %w", params.TargetKind, params.TargetID, err)
		}

		return nil
	})
	if err != nil {
		return ModerationRestriction{}, err
	}

	return saved, nil
}

// LiftModerationRestriction removes an active moderation restriction.
func (s *Service) LiftModerationRestriction(
	ctx context.Context,
	params LiftModerationRestrictionParams,
) error {
	if err := s.validateContext(ctx, "lift moderation restriction"); err != nil {
		return err
	}

	params.TargetID = strings.TrimSpace(params.TargetID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.TargetAccountID = strings.TrimSpace(params.TargetAccountID)
	params.Reason = strings.TrimSpace(params.Reason)
	if params.TargetKind == ModerationTargetKindUnspecified || params.TargetID == "" || params.ActorAccountID == "" || params.TargetAccountID == "" {
		return ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	return s.runTx(ctx, func(tx Store) error {
		conversation, err := s.lookupModerationConversation(ctx, tx, params.TargetKind, params.TargetID)
		if err != nil {
			return err
		}

		member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.ActorAccountID)
		if err != nil {
			return fmt.Errorf(
				"authorize restriction removal for account %s in conversation %s: %w",
				params.ActorAccountID,
				conversation.ID,
				err,
			)
		}
		if !s.canModerate(member, params.ActorAccountID) {
			return ErrForbidden
		}

		if err := tx.DeleteModerationRestriction(ctx, params.TargetKind, params.TargetID, params.TargetAccountID); err != nil {
			return fmt.Errorf(
				"delete moderation restriction for %s in %s:%s: %w",
				params.TargetAccountID,
				params.TargetKind,
				params.TargetID,
				err,
			)
		}

		actionID, idErr := newID("mod")
		if idErr != nil {
			return fmt.Errorf("generate moderation action id: %w", idErr)
		}
		if _, err := tx.SaveModerationAction(ctx, ModerationAction{
			ID:              actionID,
			TargetKind:      params.TargetKind,
			TargetID:        params.TargetID,
			ActorAccountID:  params.ActorAccountID,
			TargetAccountID: params.TargetAccountID,
			Type:            moderationLiftActionType(params.TargetKind),
			Reason:          params.Reason,
			Metadata: map[string]string{
				"state": "lifted",
			},
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("log moderation restriction lift for %s:%s: %w", params.TargetKind, params.TargetID, err)
		}

		return nil
	})
}

// ModerationRestrictionByTargetAndAccount resolves the active restriction for an account.
func (s *Service) ModerationRestrictionByTargetAndAccount(
	ctx context.Context,
	targetKind ModerationTargetKind,
	targetID string,
	accountID string,
) (ModerationRestriction, error) {
	if err := s.validateContext(ctx, "load moderation restriction"); err != nil {
		return ModerationRestriction{}, err
	}

	targetID = strings.TrimSpace(targetID)
	accountID = strings.TrimSpace(accountID)
	if targetKind == ModerationTargetKindUnspecified || targetID == "" || accountID == "" {
		return ModerationRestriction{}, ErrInvalidInput
	}

	restriction, err := s.store.ModerationRestrictionByTargetAndAccount(ctx, targetKind, targetID, accountID)
	if err != nil {
		return ModerationRestriction{}, fmt.Errorf("load moderation restriction for %s:%s and account %s: %w", targetKind, targetID, accountID, err)
	}
	now := s.currentTime()
	if !restriction.ExpiresAt.IsZero() && !now.Before(restriction.ExpiresAt) {
		return ModerationRestriction{}, ErrNotFound
	}

	return restriction, nil
}

// ModerationRestrictionsByTarget lists restrictions for one target.
func (s *Service) ModerationRestrictionsByTarget(
	ctx context.Context,
	targetKind ModerationTargetKind,
	targetID string,
) ([]ModerationRestriction, error) {
	if err := s.validateContext(ctx, "list moderation restrictions"); err != nil {
		return nil, err
	}

	targetID = strings.TrimSpace(targetID)
	if targetKind == ModerationTargetKindUnspecified || targetID == "" {
		return nil, ErrInvalidInput
	}

	restrictions, err := s.store.ModerationRestrictionsByTarget(ctx, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("list moderation restrictions for %s:%s: %w", targetKind, targetID, err)
	}
	now := s.currentTime()
	filtered := restrictions[:0]
	for _, restriction := range restrictions {
		if !restriction.ExpiresAt.IsZero() && !now.Before(restriction.ExpiresAt) {
			continue
		}
		filtered = append(filtered, restriction)
	}

	return filtered, nil
}

func restrictionActionType(state ModerationRestrictionState) ModerationActionType {
	switch state {
	case ModerationRestrictionStateMuted:
		return ModerationActionTypeMute
	case ModerationRestrictionStateBanned:
		return ModerationActionTypeBan
	case ModerationRestrictionStateShadowed:
		return ModerationActionTypeShadowBan
	default:
		return ModerationActionTypeUnspecified
	}
}

func moderationLiftActionType(targetKind ModerationTargetKind) ModerationActionType {
	_ = targetKind
	return ModerationActionTypeUnmute
}

func timeOrZero(duration time.Duration, base time.Time) time.Time {
	if duration <= 0 {
		return time.Time{}
	}

	return base.Add(duration)
}
