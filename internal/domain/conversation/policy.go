package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const moderationTargetSeparator = "\x1f"

func moderationKey(conversationID string, topicID string) string {
	conversationID = strings.TrimSpace(conversationID)
	topicID = strings.TrimSpace(topicID)
	if topicID == "" {
		return conversationID
	}

	return conversationID + moderationTargetSeparator + topicID
}

func splitModerationKey(targetID string) (string, string) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return "", ""
	}

	conversationID, topicID, found := strings.Cut(targetID, moderationTargetSeparator)
	if !found {
		return targetID, ""
	}

	return conversationID, topicID
}

// moderationTarget returns the moderation scope for a conversation write.
func moderationTarget(conversation Conversation, threadID string) (ModerationTargetKind, string) {
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		return ModerationTargetKindTopic, moderationKey(conversation.ID, threadID)
	}

	switch conversation.Kind {
	case ConversationKindChannel:
		return ModerationTargetKindChannel, conversation.ID
	default:
		return ModerationTargetKindConversation, conversation.ID
	}
}

// baseModerationPolicy projects conversation settings into the moderation policy model.
func baseModerationPolicy(conversation Conversation, targetKind ModerationTargetKind, targetID string) ModerationPolicy {
	return ModerationPolicy{
		TargetKind:               targetKind,
		TargetID:                 targetID,
		OnlyAdminsCanWrite:       conversation.Settings.OnlyAdminsCanWrite,
		OnlyAdminsCanAddMembers:  conversation.Settings.OnlyAdminsCanAddMembers,
		AllowReactions:           conversation.Settings.AllowReactions,
		AllowForwards:            conversation.Settings.AllowForwards,
		AllowThreads:             conversation.Settings.AllowThreads,
		RequireEncryptedMessages: conversation.Settings.RequireEncryptedMessages,
		RequireJoinApproval:      conversation.Settings.RequireJoinApproval,
		PinnedMessagesOnlyAdmins: conversation.Settings.PinnedMessagesOnlyAdmins,
		SlowModeInterval:         conversation.Settings.SlowModeInterval,
		CreatedAt:                conversation.CreatedAt,
		UpdatedAt:                conversation.UpdatedAt,
	}
}

// policyForConversation resolves the effective moderation policy for a conversation target.
func (s *Service) policyForConversation(
	ctx context.Context,
	store Store,
	conversation Conversation,
	threadID string,
) (ModerationPolicy, error) {
	targetKind, targetID := moderationTarget(conversation, threadID)
	base := baseModerationPolicy(conversation, targetKind, targetID)
	policy, err := store.ModerationPolicyByTarget(ctx, targetKind, targetID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if targetKind == ModerationTargetKindTopic {
				fallback, fallbackErr := store.ModerationPolicyByTarget(ctx, ModerationTargetKindConversation, conversation.ID)
				if fallbackErr == nil {
					return fallback, nil
				}
				if !errors.Is(fallbackErr, ErrNotFound) {
					return ModerationPolicy{}, fmt.Errorf(
						"load moderation policy for %s:%s: %w",
						ModerationTargetKindConversation,
						conversation.ID,
						fallbackErr,
					)
				}
			}

			return base, nil
		}

		return ModerationPolicy{}, fmt.Errorf("load moderation policy for %s:%s: %w", targetKind, targetID, err)
	}

	return policy, nil
}

func (s *Service) existingModerationPolicy(
	ctx context.Context,
	store Store,
	conversation Conversation,
	targetKind ModerationTargetKind,
	targetID string,
) (ModerationPolicy, bool, error) {
	policy, err := store.ModerationPolicyByTarget(ctx, targetKind, targetID)
	if err == nil {
		return policy, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return ModerationPolicy{}, false, fmt.Errorf(
			"load moderation policy for %s:%s: %w",
			targetKind,
			targetID,
			err,
		)
	}

	switch targetKind {
	case ModerationTargetKindTopic:
		_, topicID := splitModerationKey(targetID)
		inherited, inheritedErr := s.policyForConversation(ctx, store, conversation, topicID)
		if inheritedErr != nil {
			return ModerationPolicy{}, false, inheritedErr
		}

		return inherited, false, nil
	case ModerationTargetKindConversation, ModerationTargetKindChannel:
		return baseModerationPolicy(conversation, targetKind, targetID), false, nil
	default:
		return ModerationPolicy{}, false, ErrInvalidInput
	}
}

func preserveInheritedTopicThreads(params SetModerationPolicyParams, inherited ModerationPolicy) bool {
	if params.TargetKind != ModerationTargetKindTopic {
		return false
	}
	if params.AllowThreads || !inherited.AllowThreads {
		return false
	}

	return params.OnlyAdminsCanWrite ||
		params.OnlyAdminsCanAddMembers ||
		params.AllowReactions ||
		params.AllowForwards ||
		params.RequireEncryptedMessages ||
		params.RequireJoinApproval ||
		params.PinnedMessagesOnlyAdmins ||
		params.SlowModeInterval != 0 ||
		params.AntiSpamWindow != 0 ||
		params.AntiSpamBurstLimit != 0 ||
		params.ShadowMode
}

// SetModerationPolicy persists a moderation policy override.
func (s *Service) SetModerationPolicy(
	ctx context.Context,
	params SetModerationPolicyParams,
) (ModerationPolicy, error) {
	if err := s.validateContext(ctx, "set moderation policy"); err != nil {
		return ModerationPolicy{}, err
	}

	params.TargetID = strings.TrimSpace(params.TargetID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	if params.TargetKind == ModerationTargetKindUnspecified || params.TargetID == "" || params.ActorAccountID == "" {
		return ModerationPolicy{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var saved ModerationPolicy
	err := s.runTx(ctx, func(tx Store) error {
		conversation, err := s.lookupModerationConversation(ctx, tx, params.TargetKind, params.TargetID)
		if err != nil {
			return err
		}
		inherited, existing, err := s.existingModerationPolicy(ctx, tx, conversation, params.TargetKind, params.TargetID)
		if err != nil {
			return err
		}
		member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.ActorAccountID)
		if err != nil {
			return fmt.Errorf(
				"authorize policy update for account %s in conversation %s: %w",
				params.ActorAccountID,
				conversation.ID,
				err,
			)
		}
		if !s.canModerate(member, params.ActorAccountID) {
			return ErrForbidden
		}

		allowThreads := params.AllowThreads
		if preserveInheritedTopicThreads(params, inherited) {
			allowThreads = inherited.AllowThreads
		}

		policy := ModerationPolicy{
			TargetKind:               params.TargetKind,
			TargetID:                 params.TargetID,
			OnlyAdminsCanWrite:       params.OnlyAdminsCanWrite,
			OnlyAdminsCanAddMembers:  params.OnlyAdminsCanAddMembers,
			AllowReactions:           params.AllowReactions,
			AllowForwards:            params.AllowForwards,
			AllowThreads:             allowThreads,
			RequireEncryptedMessages: params.RequireEncryptedMessages,
			RequireJoinApproval:      params.RequireJoinApproval,
			PinnedMessagesOnlyAdmins: params.PinnedMessagesOnlyAdmins,
			SlowModeInterval:         params.SlowModeInterval,
			AntiSpamWindow:           params.AntiSpamWindow,
			AntiSpamBurstLimit:       params.AntiSpamBurstLimit,
			ShadowMode:               params.ShadowMode,
			CreatedAt:                now,
			UpdatedAt:                now,
		}
		if existing {
			policy.CreatedAt = inherited.CreatedAt
		}
		saved, err = tx.SaveModerationPolicy(ctx, policy)
		if err != nil {
			return fmt.Errorf("save moderation policy for %s:%s: %w", params.TargetKind, params.TargetID, err)
		}

		actionID, idErr := newID("mod")
		if idErr != nil {
			return fmt.Errorf("generate moderation action id: %w", idErr)
		}

		if _, _, err := s.saveModerationActionWithSyncEvent(ctx, tx, conversation.ID, ModerationAction{
			ID:             actionID,
			TargetKind:     params.TargetKind,
			TargetID:       params.TargetID,
			ActorAccountID: params.ActorAccountID,
			Type:           ModerationActionTypePolicySet,
			Metadata: map[string]string{
				"target_kind": string(params.TargetKind),
			},
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("log moderation policy action for %s:%s: %w", params.TargetKind, params.TargetID, err)
		}

		return nil
	})
	if err != nil {
		return ModerationPolicy{}, err
	}

	return saved, nil
}

// ModerationPolicyByTarget loads the effective policy override for a target.
func (s *Service) ModerationPolicyByTarget(
	ctx context.Context,
	targetKind ModerationTargetKind,
	targetID string,
) (ModerationPolicy, error) {
	if err := s.validateContext(ctx, "load moderation policy"); err != nil {
		return ModerationPolicy{}, err
	}

	targetKind = ModerationTargetKind(strings.TrimSpace(string(targetKind)))
	targetID = strings.TrimSpace(targetID)
	if targetKind == "" || targetID == "" {
		return ModerationPolicy{}, ErrInvalidInput
	}

	policy, err := s.store.ModerationPolicyByTarget(ctx, ModerationTargetKind(targetKind), targetID)
	if err != nil {
		return ModerationPolicy{}, fmt.Errorf("load moderation policy for %s:%s: %w", targetKind, targetID, err)
	}

	return policy, nil
}

func (s *Service) lookupModerationConversation(ctx context.Context, store Store, targetKind ModerationTargetKind, targetID string) (Conversation, error) {
	switch targetKind {
	case ModerationTargetKindConversation, ModerationTargetKindChannel:
		conversation, err := store.ConversationByID(ctx, targetID)
		if err != nil {
			return Conversation{}, fmt.Errorf("load moderation conversation %s: %w", targetID, err)
		}
		if targetKind == ModerationTargetKindChannel && conversation.Kind != ConversationKindChannel {
			return Conversation{}, ErrInvalidInput
		}

		return conversation, nil
	case ModerationTargetKindTopic:
		conversationID, topicID := splitModerationKey(targetID)
		if conversationID == "" || topicID == "" {
			return Conversation{}, ErrInvalidInput
		}
		conversation, err := store.ConversationByID(ctx, conversationID)
		if err != nil {
			return Conversation{}, fmt.Errorf("load moderation conversation %s: %w", conversationID, err)
		}
		topic, err := store.TopicByConversationAndID(ctx, conversation.ID, topicID)
		if err != nil {
			return Conversation{}, fmt.Errorf("load moderation topic %s in conversation %s: %w", topicID, conversation.ID, err)
		}
		if topic.ConversationID != conversation.ID {
			return Conversation{}, ErrInvalidInput
		}

		return conversation, nil
	default:
		return Conversation{}, ErrInvalidInput
	}
}

func (s *Service) canModerate(member ConversationMember, actorAccountID string) bool {
	if member.AccountID != actorAccountID {
		return false
	}

	switch member.Role {
	case MemberRoleOwner, MemberRoleAdmin:
		return true
	default:
		return false
	}
}
