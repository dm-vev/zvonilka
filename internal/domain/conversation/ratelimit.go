package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CheckModerationWrite evaluates message write eligibility.
func (s *Service) CheckModerationWrite(
	ctx context.Context,
	params CheckModerationWriteParams,
) (ModerationDecision, error) {
	return s.checkModerationWrite(ctx, s.store, params)
}

func (s *Service) checkModerationWrite(
	ctx context.Context,
	store Store,
	params CheckModerationWriteParams,
) (ModerationDecision, error) {
	if err := s.validateContext(ctx, "check moderation write"); err != nil {
		return ModerationDecision{}, err
	}
	if store == nil {
		return ModerationDecision{}, ErrInvalidInput
	}

	params.TargetID = strings.TrimSpace(params.TargetID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	if params.TargetKind == ModerationTargetKindUnspecified || params.TargetID == "" || params.ActorAccountID == "" {
		return ModerationDecision{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	restriction, err := store.ModerationRestrictionByTargetAndAccount(ctx, params.TargetKind, params.TargetID, params.ActorAccountID)
	if err != nil && err != ErrNotFound {
		return ModerationDecision{}, fmt.Errorf(
			"load moderation restriction for %s:%s and account %s: %w",
			params.TargetKind,
			params.TargetID,
			params.ActorAccountID,
			err,
		)
	}
	if !restriction.ExpiresAt.IsZero() && !now.Before(restriction.ExpiresAt) {
		restriction = ModerationRestriction{}
	}
	if restriction.State == ModerationRestrictionStateUnspecified && params.TargetKind == ModerationTargetKindTopic {
		conversationID, topicID := splitModerationKey(params.TargetID)
		if conversationID != "" && topicID != "" {
			fallback, fallbackErr := store.ModerationRestrictionByTargetAndAccount(
				ctx,
				ModerationTargetKindConversation,
				conversationID,
				params.ActorAccountID,
			)
			if fallbackErr != nil && fallbackErr != ErrNotFound {
				return ModerationDecision{}, fmt.Errorf(
					"load moderation restriction for %s:%s and account %s: %w",
					ModerationTargetKindConversation,
					conversationID,
					params.ActorAccountID,
					fallbackErr,
				)
			}
			if !fallback.ExpiresAt.IsZero() && !now.Before(fallback.ExpiresAt) {
				fallback = ModerationRestriction{}
			}
			restriction = fallback
		}
	}

	switch restriction.State {
	case ModerationRestrictionStateBanned, ModerationRestrictionStateMuted:
		return ModerationDecision{}, ErrForbidden
	case ModerationRestrictionStateShadowed:
		return ModerationDecision{Allowed: true, ShadowHidden: true}, nil
	}

	if params.BasePolicy.OnlyAdminsCanWrite && params.ActorRole != MemberRoleOwner && params.ActorRole != MemberRoleAdmin {
		return ModerationDecision{}, ErrForbidden
	}

	if params.BasePolicy.SlowModeInterval > 0 || params.BasePolicy.AntiSpamWindow > 0 || params.BasePolicy.AntiSpamBurstLimit > 0 {
		rateState, err := store.ModerationRateStateByTargetAndAccount(ctx, params.TargetKind, params.TargetID, params.ActorAccountID)
		if err != nil && err != ErrNotFound {
			return ModerationDecision{}, fmt.Errorf(
				"load moderation rate state for %s:%s and account %s: %w",
				params.TargetKind,
				params.TargetID,
				params.ActorAccountID,
				err,
			)
		}

		if params.BasePolicy.SlowModeInterval > 0 && !rateState.LastWriteAt.IsZero() {
			nextAllowed := rateState.LastWriteAt.Add(params.BasePolicy.SlowModeInterval)
			if now.Before(nextAllowed) {
				return ModerationDecision{
					RetryAfter: nextAllowed.Sub(now),
				}, ErrRateLimited
			}
		}

		if params.BasePolicy.AntiSpamWindow > 0 && params.BasePolicy.AntiSpamBurstLimit > 0 {
			if rateState.WindowStartedAt.IsZero() || now.Sub(rateState.WindowStartedAt) >= params.BasePolicy.AntiSpamWindow {
				rateState.WindowStartedAt = now
				rateState.WindowCount = 0
			}
			if rateState.WindowCount >= uint64(params.BasePolicy.AntiSpamBurstLimit) {
				nextAllowed := rateState.WindowStartedAt.Add(params.BasePolicy.AntiSpamWindow)
				return ModerationDecision{
					RetryAfter: nextAllowed.Sub(now),
				}, ErrRateLimited
			}
		}
	}

	return ModerationDecision{Allowed: true}, nil
}

// RecordModerationWrite updates the moderation rate state after a successful write.
func (s *Service) RecordModerationWrite(
	ctx context.Context,
	params RecordModerationWriteParams,
) (ModerationRateState, error) {
	if err := s.validateContext(ctx, "record moderation write"); err != nil {
		return ModerationRateState{}, err
	}

	params.TargetID = strings.TrimSpace(params.TargetID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	if params.TargetKind == ModerationTargetKindUnspecified || params.TargetID == "" || params.ActorAccountID == "" {
		return ModerationRateState{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	state, err := s.store.ModerationRateStateByTargetAndAccount(ctx, params.TargetKind, params.TargetID, params.ActorAccountID)
	if err != nil && err != ErrNotFound {
		return ModerationRateState{}, fmt.Errorf(
			"load moderation rate state for %s:%s and account %s: %w",
			params.TargetKind,
			params.TargetID,
			params.ActorAccountID,
			err,
		)
	}

	if state.TargetID == "" {
		state.TargetKind = params.TargetKind
		state.TargetID = params.TargetID
		state.AccountID = params.ActorAccountID
	}

	state.LastWriteAt = now
	window := params.AntiSpamWindow
	if window <= 0 {
		window = time.Hour
	}
	if state.WindowStartedAt.IsZero() || now.Sub(state.WindowStartedAt) >= window {
		state.WindowStartedAt = now
		state.WindowCount = 1
	} else {
		state.WindowCount++
	}
	state.UpdatedAt = now

	saved, err := s.store.SaveModerationRateState(ctx, state)
	if err != nil {
		return ModerationRateState{}, fmt.Errorf(
			"save moderation rate state for %s:%s and account %s: %w",
			params.TargetKind,
			params.TargetID,
			params.ActorAccountID,
			err,
		)
	}

	return saved, nil
}
