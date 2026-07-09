package conversation

import (
	"context"
	"fmt"
	"strings"
)

// ListMessages returns the messages in a conversation.
func (s *Service) ListMessages(ctx context.Context, params ListMessagesParams) ([]Message, error) {
	if err := s.validateContext(ctx, "list messages"); err != nil {
		return nil, err
	}
	now := s.currentTime()
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.ThreadID = strings.TrimSpace(params.ThreadID)
	if params.ConversationID == "" || params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	member, err := s.store.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.AccountID)
	if err != nil {
		return nil, fmt.Errorf("authorize conversation %s for account %s: %w", params.ConversationID, params.AccountID, err)
	}
	if !isActiveMember(member) {
		return nil, ErrForbidden
	}
	conversation, err := s.store.ConversationByID(ctx, params.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("load conversation %s: %w", params.ConversationID, err)
	}
	if params.ThreadID != "" {
		policy, err := s.policyForConversation(ctx, s.store, conversation, params.ThreadID)
		if err != nil {
			return nil, err
		}
		if conversation.Kind != ConversationKindGroup || !policy.AllowThreads {
			return nil, ErrForbidden
		}
		if _, err := s.store.TopicByConversationAndID(ctx, params.ConversationID, params.ThreadID); err != nil {
			return nil, fmt.Errorf("load topic %s in conversation %s: %w", params.ThreadID, params.ConversationID, err)
		}
	}

	shadowedAccounts := make(map[string]struct{})
	targetKind, targetID := moderationTarget(conversation, params.ThreadID)
	collectRestrictions := func(restrictions []ModerationRestriction) {
		for _, restriction := range restrictions {
			if restriction.State != ModerationRestrictionStateShadowed {
				continue
			}
			if !restriction.ExpiresAt.IsZero() && !now.Before(restriction.ExpiresAt) {
				continue
			}
			shadowedAccounts[restriction.AccountID] = struct{}{}
		}
	}
	restrictions, err := s.store.ModerationRestrictionsByTarget(ctx, targetKind, targetID)
	if err != nil {
		return nil, fmt.Errorf("load moderation restrictions for %s:%s: %w", targetKind, targetID, err)
	}
	collectRestrictions(restrictions)
	if params.ThreadID != "" {
		fallbackRestrictions, fallbackErr := s.store.ModerationRestrictionsByTarget(ctx, ModerationTargetKindConversation, conversation.ID)
		if fallbackErr != nil {
			return nil, fmt.Errorf("load moderation restrictions for %s:%s: %w", ModerationTargetKindConversation, conversation.ID, fallbackErr)
		}
		collectRestrictions(fallbackRestrictions)
	}

	limit := clampLimit(params.Limit, 100, 500)
	messages, err := s.store.MessagesByConversationID(ctx, params.ConversationID, params.ThreadID, params.FromSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("load messages for conversation %s: %w", params.ConversationID, err)
	}

	filtered := messages[:0]
	for _, message := range messages {
		if !params.IncludeDeleted && !message.DeletedAt.IsZero() {
			continue
		}
		if _, shadowed := shadowedAccounts[message.SenderAccountID]; shadowed && message.SenderAccountID != params.AccountID {
			continue
		}
		filtered = append(filtered, message)
	}

	return filtered, nil
}

// ListScheduledMessages returns the sender's pending or failed scheduled messages.
func (s *Service) ListScheduledMessages(ctx context.Context, params ListScheduledMessagesParams) ([]Message, error) {
	if err := s.validateContext(ctx, "list scheduled messages"); err != nil {
		return nil, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.ThreadID = strings.TrimSpace(params.ThreadID)
	if params.ConversationID == "" || params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	member, err := s.store.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.AccountID)
	if err != nil {
		return nil, fmt.Errorf(
			"authorize scheduled message list for conversation %s and account %s: %w",
			params.ConversationID,
			params.AccountID,
			err,
		)
	}
	if !isActiveMember(member) {
		return nil, ErrForbidden
	}

	if params.ThreadID != "" {
		if _, err := s.store.TopicByConversationAndID(ctx, params.ConversationID, params.ThreadID); err != nil {
			return nil, fmt.Errorf("load topic %s in conversation %s: %w", params.ThreadID, params.ConversationID, err)
		}
	}

	limit := clampLimit(params.Limit, 100, 500)
	messages, err := s.store.ScheduledMessagesByConversationAndSender(
		ctx,
		params.ConversationID,
		params.AccountID,
		params.ThreadID,
		params.IncludeFailed,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("load scheduled messages for conversation %s: %w", params.ConversationID, err)
	}

	return messages, nil
}

// SendMessage persists a new message, emits an event, and advances conversation state.
func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) (Message, EventEnvelope, error) {
	if err := s.validateContext(ctx, "send message"); err != nil {
		return Message{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.SenderAccountID = strings.TrimSpace(params.SenderAccountID)
	params.SenderDeviceID = strings.TrimSpace(params.SenderDeviceID)
	params.Draft = normalizeMessageDraft(params.Draft)
	params.Draft.ThreadID = strings.TrimSpace(params.Draft.ThreadID)
	if params.ConversationID == "" || params.SenderAccountID == "" || params.SenderDeviceID == "" {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}
	if params.Draft.Kind == MessageKindUnspecified {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}

	draft := params.Draft
	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedMessage Message
		savedEvent   EventEnvelope
		decision     ModerationDecision
	)
	scheduled := !draft.DeliverAt.IsZero() && draft.DeliverAt.After(now)

	err := s.runTx(ctx, func(tx Store) error {
		conversation, err := tx.ConversationByID(ctx, params.ConversationID)
		if err != nil {
			return fmt.Errorf("load conversation %s: %w", params.ConversationID, err)
		}
		member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.SenderAccountID)
		if err != nil {
			return fmt.Errorf("authorize sender %s for conversation %s: %w", params.SenderAccountID, conversation.ID, err)
		}
		if !isActiveMember(member) {
			return ErrForbidden
		}
		if len(draft.MentionAccountIDs) > 0 {
			members, loadErr := tx.ConversationMembersByConversationID(ctx, conversation.ID)
			if loadErr != nil {
				return fmt.Errorf("load members for conversation %s: %w", conversation.ID, loadErr)
			}
			if err := validateMentionTargets(members, draft.MentionAccountIDs); err != nil {
				return err
			}
		}

		topicID := draft.ThreadID
		if topicID != "" {
			policy, policyErr := s.policyForConversation(ctx, tx, conversation, topicID)
			if policyErr != nil {
				return policyErr
			}
			if conversation.Kind != ConversationKindGroup || !policy.AllowThreads {
				return ErrForbidden
			}
		}
		topic, topicErr := tx.TopicByConversationAndID(ctx, conversation.ID, topicID)
		if topicErr != nil {
			return fmt.Errorf("load topic %s in conversation %s: %w", topicID, conversation.ID, topicErr)
		}
		if topic.Archived || topic.Closed {
			return ErrForbidden
		}

		replyTo := draft.ReplyTo
		if replyTo.MessageID != "" {
			replyTarget, replyErr := tx.MessageByID(ctx, conversation.ID, replyTo.MessageID)
			if replyErr != nil {
				return fmt.Errorf("load reply target %s in conversation %s: %w", replyTo.MessageID, conversation.ID, replyErr)
			}
			if !replyTarget.DeletedAt.IsZero() || replyTarget.Status == MessageStatusDeleted {
				return ErrConflict
			}
			if strings.TrimSpace(replyTo.ConversationID) != "" && strings.TrimSpace(replyTo.ConversationID) != conversation.ID {
				return ErrConflict
			}
			if strings.TrimSpace(replyTarget.ThreadID) != topic.ID {
				return ErrConflict
			}
			replyTo = MessageReference{
				ConversationID:  conversation.ID,
				MessageID:       replyTarget.ID,
				SenderAccountID: replyTarget.SenderAccountID,
				MessageKind:     replyTarget.Kind,
				Snippet:         strings.TrimSpace(replyTo.Snippet),
			}
		}

		messageID, err := newID("msg")
		if err != nil {
			return fmt.Errorf("generate message id: %w", err)
		}

		message := Message{
			ID:                  messageID,
			ConversationID:      conversation.ID,
			SenderAccountID:     params.SenderAccountID,
			SenderDeviceID:      params.SenderDeviceID,
			ClientMessageID:     strings.TrimSpace(draft.ClientMessageID),
			Kind:                draft.Kind,
			Status:              MessageStatusSent,
			Payload:             draft.Payload,
			Attachments:         append([]AttachmentRef(nil), draft.Attachments...),
			MentionAccountIDs:   append([]string(nil), draft.MentionAccountIDs...),
			ReplyTo:             replyTo,
			ThreadID:            strings.TrimSpace(draft.ThreadID),
			Silent:              draft.Silent,
			Pinned:              false,
			DisableLinkPreviews: draft.DisableLinkPreviews,
			DeliverAt:           draft.DeliverAt,
			Metadata:            trimMetadata(draft.Metadata),
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		if scheduled {
			message.Status = MessageStatusPending
			savedMessage, err = tx.SaveMessage(ctx, message)
			if err != nil {
				return fmt.Errorf("save scheduled message: %w", err)
			}

			return nil
		}

		publishState, publishDecision, publishErr := s.validateMessageForPublication(ctx, tx, message, now)
		if publishErr != nil {
			return publishErr
		}
		decision = publishDecision

		savedMessage, savedEvent, err = s.publishMessage(ctx, tx, publishMessageParams{
			Message:        message,
			State:          publishState,
			Decision:       publishDecision,
			ActorAccountID: params.SenderAccountID,
			ActorDeviceID:  params.SenderDeviceID,
			CausationID:    params.CausationID,
			CorrelationID:  params.CorrelationID,
			PublishedAt:    now,
		})
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return Message{}, EventEnvelope{}, err
	}

	if !scheduled {
		if !decision.ShadowHidden {
			s.indexMessage(ctx, savedMessage)
		}
		s.indexConversationByID(ctx, savedMessage.ConversationID)
	}

	return savedMessage, savedEvent, nil
}
