package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type messageActionState struct {
	conversation Conversation
	member       ConversationMember
	message      Message
	topic        ConversationTopic
}

func (s *Service) loadMessageActionState(
	ctx context.Context,
	store Store,
	operation string,
	conversationID string,
	messageID string,
	actorAccountID string,
) (messageActionState, error) {
	if err := s.validateContext(ctx, operation); err != nil {
		return messageActionState{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	messageID = strings.TrimSpace(messageID)
	actorAccountID = strings.TrimSpace(actorAccountID)
	if conversationID == "" || messageID == "" || actorAccountID == "" {
		return messageActionState{}, ErrInvalidInput
	}

	conversation, err := store.ConversationByID(ctx, conversationID)
	if err != nil {
		return messageActionState{}, fmt.Errorf("load conversation %s: %w", conversationID, err)
	}

	member, err := store.ConversationMemberByConversationAndAccount(ctx, conversation.ID, actorAccountID)
	if err != nil {
		return messageActionState{}, fmt.Errorf(
			"authorize actor %s for conversation %s: %w",
			actorAccountID,
			conversation.ID,
			err,
		)
	}
	if !isActiveMember(member) {
		return messageActionState{}, ErrForbidden
	}

	message, err := store.MessageByID(ctx, conversation.ID, messageID)
	if err != nil {
		return messageActionState{}, fmt.Errorf("load message %s in conversation %s: %w", messageID, conversation.ID, err)
	}

	topicID := strings.TrimSpace(message.ThreadID)
	topic, err := store.TopicByConversationAndID(ctx, conversation.ID, topicID)
	if err != nil {
		return messageActionState{}, fmt.Errorf("load topic %s in conversation %s: %w", topicID, conversation.ID, err)
	}

	return messageActionState{
		conversation: conversation,
		member:       member,
		message:      message,
		topic:        topic,
	}, nil
}

func (s *Service) finalizeMessageAction(
	ctx context.Context,
	tx Store,
	state messageActionState,
	now time.Time,
	event EventEnvelope,
) error {
	state.topic.LastSequence = event.Sequence
	state.topic.UpdatedAt = now
	if _, err := tx.SaveTopic(ctx, state.topic); err != nil {
		return fmt.Errorf("update topic %s after message action: %w", state.topic.ID, err)
	}

	state.conversation.LastSequence = event.Sequence
	state.conversation.UpdatedAt = now
	if _, err := tx.SaveConversation(ctx, state.conversation); err != nil {
		return fmt.Errorf("update conversation %s after message action: %w", state.conversation.ID, err)
	}

	return nil
}

func canEditMessage(member ConversationMember, message Message) bool {
	if member.AccountID == message.SenderAccountID {
		return true
	}

	return member.Role == MemberRoleOwner || member.Role == MemberRoleAdmin
}

func canDeleteMessage(member ConversationMember, message Message) bool {
	return canEditMessage(member, message)
}

func canPinMessage(member ConversationMember, policy ModerationPolicy) bool {
	if policy.PinnedMessagesOnlyAdmins {
		return member.Role == MemberRoleOwner || member.Role == MemberRoleAdmin
	}

	return true
}

// EditMessage edits an existing message in place.
func (s *Service) EditMessage(ctx context.Context, params EditMessageParams) (Message, EventEnvelope, error) {
	if err := s.validateContext(ctx, "edit message"); err != nil {
		return Message{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.ActorDeviceID = strings.TrimSpace(params.ActorDeviceID)
	params.Draft = normalizeMessageDraft(params.Draft)
	if params.ConversationID == "" || params.MessageID == "" || params.ActorAccountID == "" {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}
	if params.Draft.Kind == MessageKindUnspecified {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}
	now := params.EditedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedMessage Message
		savedEvent   EventEnvelope
	)

	err := s.runTx(ctx, func(tx Store) error {
		state, err := s.loadMessageActionState(ctx, tx, "load message for edit", params.ConversationID, params.MessageID, params.ActorAccountID)
		if err != nil {
			return err
		}
		if !canEditMessage(state.member, state.message) {
			return ErrForbidden
		}
		if !state.message.DeletedAt.IsZero() || state.message.Status == MessageStatusDeleted {
			return ErrConflict
		}
		if err := ValidateMessagePayload(
			params.Draft.Payload,
			state.conversation.Settings.RequireEncryptedMessages,
		); err != nil {
			return ErrInvalidInput
		}
		if len(params.Draft.MentionAccountIDs) > 0 {
			members, loadErr := tx.ConversationMembersByConversationID(ctx, state.conversation.ID)
			if loadErr != nil {
				return fmt.Errorf("load members for conversation %s: %w", state.conversation.ID, loadErr)
			}
			if err := validateMentionTargets(members, params.Draft.MentionAccountIDs); err != nil {
				return err
			}
		}

		nextMessage := state.message
		nextMessage.Kind = params.Draft.Kind
		nextMessage.Payload = params.Draft.Payload
		nextMessage.Attachments = append([]AttachmentRef(nil), params.Draft.Attachments...)
		nextMessage.MentionAccountIDs = append([]string(nil), params.Draft.MentionAccountIDs...)
		StripMessageHints(&nextMessage)
		nextMessage.Silent = params.Draft.Silent
		nextMessage.DisableLinkPreviews = params.Draft.DisableLinkPreviews
		nextMessage.Metadata = trimMetadata(params.Draft.Metadata)
		nextMessage.UpdatedAt = now
		nextMessage.EditedAt = now

		savedMessage, err = tx.SaveMessage(ctx, nextMessage)
		if err != nil {
			return fmt.Errorf("save edited message %s: %w", nextMessage.ID, err)
		}

		savedEvent, err = tx.SaveEvent(ctx, EventEnvelope{
			EventID:        eventID(),
			EventType:      EventTypeMessageEdited,
			ConversationID: state.conversation.ID,
			ActorAccountID: params.ActorAccountID,
			ActorDeviceID:  params.ActorDeviceID,
			MessageID:      savedMessage.ID,
			PayloadType:    "message",
			Payload:        params.Draft.Payload,
			Metadata: messageEventMetadata(savedMessage.ID, savedMessage.ThreadID, savedMessage.ReplyTo, map[string]string{
				"action": "edit",
			}),
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("save edited message event: %w", err)
		}

		if err := s.finalizeMessageAction(ctx, tx, state, now, savedEvent); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return Message{}, EventEnvelope{}, err
	}

	s.deleteMessageIndex(ctx, savedMessage)
	s.indexConversationByID(ctx, savedMessage.ConversationID)

	return savedMessage, savedEvent, nil
}

// DeleteMessage marks an existing message as deleted.
func (s *Service) DeleteMessage(ctx context.Context, params DeleteMessageParams) (Message, EventEnvelope, error) {
	if err := s.validateContext(ctx, "delete message"); err != nil {
		return Message{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.ActorDeviceID = strings.TrimSpace(params.ActorDeviceID)
	if params.ConversationID == "" || params.MessageID == "" || params.ActorAccountID == "" {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}

	now := params.DeletedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedMessage Message
		savedEvent   EventEnvelope
	)

	err := s.runTx(ctx, func(tx Store) error {
		state, err := s.loadMessageActionState(ctx, tx, "load message for delete", params.ConversationID, params.MessageID, params.ActorAccountID)
		if err != nil {
			return err
		}
		if !canDeleteMessage(state.member, state.message) {
			return ErrForbidden
		}
		if !state.message.DeletedAt.IsZero() || state.message.Status == MessageStatusDeleted {
			savedMessage = state.message
			return nil
		}

		nextMessage := state.message
		nextMessage.Status = MessageStatusDeleted
		nextMessage.Pinned = false
		nextMessage.UpdatedAt = now
		nextMessage.DeletedAt = now

		savedMessage, err = tx.SaveMessage(ctx, nextMessage)
		if err != nil {
			return fmt.Errorf("save deleted message %s: %w", nextMessage.ID, err)
		}

		savedEvent, err = tx.SaveEvent(ctx, EventEnvelope{
			EventID:        eventID(),
			EventType:      EventTypeMessageDeleted,
			ConversationID: state.conversation.ID,
			ActorAccountID: params.ActorAccountID,
			ActorDeviceID:  params.ActorDeviceID,
			MessageID:      savedMessage.ID,
			PayloadType:    "message",
			Metadata: messageEventMetadata(savedMessage.ID, savedMessage.ThreadID, savedMessage.ReplyTo, map[string]string{
				"action": "delete",
			}),
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("save deleted message event: %w", err)
		}

		if err := s.finalizeMessageAction(ctx, tx, state, now, savedEvent); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return Message{}, EventEnvelope{}, err
	}

	s.indexMessage(ctx, savedMessage)
	s.indexConversationByID(ctx, savedMessage.ConversationID)

	return savedMessage, savedEvent, nil
}

// PinMessage pins or unpins a message.
func (s *Service) PinMessage(ctx context.Context, params PinMessageParams) (Message, EventEnvelope, error) {
	if err := s.validateContext(ctx, "pin message"); err != nil {
		return Message{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.ActorDeviceID = strings.TrimSpace(params.ActorDeviceID)
	if params.ConversationID == "" || params.MessageID == "" || params.ActorAccountID == "" {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}

	now := params.UpdatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedMessage Message
		savedEvent   EventEnvelope
	)

	err := s.runTx(ctx, func(tx Store) error {
		state, err := s.loadMessageActionState(ctx, tx, "load message for pin", params.ConversationID, params.MessageID, params.ActorAccountID)
		if err != nil {
			return err
		}
		policy, err := s.policyForConversation(ctx, tx, state.conversation, state.topic.ID)
		if err != nil {
			return err
		}
		if !canPinMessage(state.member, policy) {
			return ErrForbidden
		}
		if !state.message.DeletedAt.IsZero() || state.message.Status == MessageStatusDeleted {
			return ErrConflict
		}
		if state.message.Pinned == params.Pinned {
			savedMessage = state.message
			return nil
		}

		nextMessage := state.message
		nextMessage.Pinned = params.Pinned
		nextMessage.UpdatedAt = now

		savedMessage, err = tx.SaveMessage(ctx, nextMessage)
		if err != nil {
			return fmt.Errorf("save pinned message %s: %w", nextMessage.ID, err)
		}

		savedEvent, err = tx.SaveEvent(ctx, EventEnvelope{
			EventID:        eventID(),
			EventType:      EventTypeMessagePinned,
			ConversationID: state.conversation.ID,
			ActorAccountID: params.ActorAccountID,
			ActorDeviceID:  params.ActorDeviceID,
			MessageID:      savedMessage.ID,
			PayloadType:    "message",
			Metadata: messageEventMetadata(savedMessage.ID, savedMessage.ThreadID, savedMessage.ReplyTo, map[string]string{
				"action": "pin",
				"pinned": fmt.Sprintf("%t", params.Pinned),
			}),
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("save pinned message event: %w", err)
		}

		if err := s.finalizeMessageAction(ctx, tx, state, now, savedEvent); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return Message{}, EventEnvelope{}, err
	}

	return savedMessage, savedEvent, nil
}
