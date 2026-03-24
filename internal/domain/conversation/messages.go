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
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
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

	limit := clampLimit(params.Limit, 100, 500)
	messages, err := s.store.MessagesByConversationID(ctx, params.ConversationID, params.FromSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("load messages for conversation %s: %w", params.ConversationID, err)
	}

	if params.IncludeDeleted {
		return messages, nil
	}

	filtered := messages[:0]
	for _, message := range messages {
		if message.DeletedAt.IsZero() {
			filtered = append(filtered, message)
		}
	}

	return filtered, nil
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
	if params.ConversationID == "" || params.SenderAccountID == "" || params.SenderDeviceID == "" {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}
	if params.Draft.Kind == MessageKindUnspecified {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedMessage Message
		savedEvent   EventEnvelope
	)

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
		if conversation.Settings.OnlyAdminsCanWrite && member.Role != MemberRoleOwner && member.Role != MemberRoleAdmin {
			return ErrForbidden
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
			ClientMessageID:     strings.TrimSpace(params.Draft.ClientMessageID),
			Kind:                params.Draft.Kind,
			Status:              MessageStatusSent,
			Payload:             params.Draft.Payload,
			Attachments:         append([]AttachmentRef(nil), params.Draft.Attachments...),
			ReplyTo:             params.Draft.ReplyTo,
			ThreadID:            strings.TrimSpace(params.Draft.ThreadID),
			Silent:              params.Draft.Silent,
			Pinned:              false,
			DisableLinkPreviews: params.Draft.DisableLinkPreviews,
			Metadata:            trimMetadata(params.Draft.Metadata),
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		savedMessage, err = tx.SaveMessage(ctx, message)
		if err != nil {
			return fmt.Errorf("save message: %w", err)
		}

		event := EventEnvelope{
			EventID:        eventID(),
			EventType:      EventTypeMessageCreated,
			ConversationID: conversation.ID,
			ActorAccountID: params.SenderAccountID,
			ActorDeviceID:  params.SenderDeviceID,
			CausationID:    params.CausationID,
			CorrelationID:  params.CorrelationID,
			MessageID:      savedMessage.ID,
			PayloadType:    "message",
			Payload:        params.Draft.Payload,
			Metadata:       conversationEventMetadata(savedMessage.ID, params.Draft.Metadata),
			CreatedAt:      now,
		}
		savedEvent, err = tx.SaveEvent(ctx, event)
		if err != nil {
			return fmt.Errorf("save message event: %w", err)
		}

		savedMessage.Sequence = savedEvent.Sequence
		savedMessage.UpdatedAt = now
		savedMessage, err = tx.SaveMessage(ctx, savedMessage)
		if err != nil {
			return fmt.Errorf("update message sequence: %w", err)
		}

		readState := ReadState{
			ConversationID:      conversation.ID,
			AccountID:           params.SenderAccountID,
			DeviceID:            params.SenderDeviceID,
			LastDeliveredSequence: savedEvent.Sequence,
			LastReadSequence:    savedEvent.Sequence,
			LastAckedSequence:    savedEvent.Sequence,
			UpdatedAt:           now,
		}
		if _, err := tx.SaveReadState(ctx, readState); err != nil {
			return fmt.Errorf("save sender read state: %w", err)
		}

		conversation.LastSequence = savedEvent.Sequence
		conversation.LastMessageAt = now
		conversation.UpdatedAt = now
		if _, err := tx.SaveConversation(ctx, conversation); err != nil {
			return fmt.Errorf("update conversation %s after message: %w", conversation.ID, err)
		}

		state := SyncState{
			DeviceID:            params.SenderDeviceID,
			AccountID:           params.SenderAccountID,
			LastAppliedSequence: savedEvent.Sequence,
			LastAckedSequence:   savedEvent.Sequence,
			ServerTime:          now,
		}
		if _, err := tx.SaveSyncState(ctx, state); err != nil {
			return fmt.Errorf("save sync state: %w", err)
		}

		return nil
	})
	if err != nil {
		return Message{}, EventEnvelope{}, err
	}

	return savedMessage, savedEvent, nil
}
