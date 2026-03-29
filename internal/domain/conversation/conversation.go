package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// UpdateConversation applies mutable metadata and settings changes to a conversation.
func (s *Service) UpdateConversation(ctx context.Context, params UpdateConversationParams) (Conversation, error) {
	if err := s.validateContext(ctx, "update conversation"); err != nil {
		return Conversation{}, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	if params.ConversationID == "" || params.ActorAccountID == "" {
		return Conversation{}, ErrInvalidInput
	}

	updatedAt := params.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = s.currentTime()
	}

	var saved Conversation
	err := s.runTx(ctx, func(tx Store) error {
		conversationRow, actorMember, err := s.loadConversationActor(ctx, tx, params.ConversationID, params.ActorAccountID)
		if err != nil {
			return err
		}
		if !canManageConversation(actorMember, params.ActorAccountID, conversationRow.Kind) {
			return ErrForbidden
		}

		if params.Title != nil {
			conversationRow.Title = strings.TrimSpace(*params.Title)
		}
		if params.Description != nil {
			conversationRow.Description = strings.TrimSpace(*params.Description)
		}
		if params.AvatarMediaID != nil {
			conversationRow.AvatarMediaID = strings.TrimSpace(*params.AvatarMediaID)
		}
		if params.Settings != nil {
			conversationRow.Settings = normalizeConversationSettings(*params.Settings)
		}
		conversationRow.UpdatedAt = updatedAt

		saved, err = tx.SaveConversation(ctx, conversationRow)
		if err != nil {
			return fmt.Errorf("save conversation %s: %w", conversationRow.ID, err)
		}
		saved, _, err = s.appendConversationEvent(ctx, tx, saved, params.ActorAccountID, "", EventTypeConversationUpdated, "conversation", map[string]string{
			"scope": "conversation",
		}, updatedAt)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return Conversation{}, err
	}

	s.indexConversationByID(ctx, saved.ID)
	return saved, nil
}

func (s *Service) loadConversationActor(
	ctx context.Context,
	store Store,
	conversationID string,
	accountID string,
) (Conversation, ConversationMember, error) {
	conversationID = strings.TrimSpace(conversationID)
	accountID = strings.TrimSpace(accountID)
	if conversationID == "" || accountID == "" {
		return Conversation{}, ConversationMember{}, ErrInvalidInput
	}

	conversationRow, err := store.ConversationByID(ctx, conversationID)
	if err != nil {
		return Conversation{}, ConversationMember{}, fmt.Errorf("load conversation %s: %w", conversationID, err)
	}

	member, err := store.ConversationMemberByConversationAndAccount(ctx, conversationID, accountID)
	if err != nil {
		return Conversation{}, ConversationMember{}, fmt.Errorf(
			"authorize conversation %s for account %s: %w",
			conversationID,
			accountID,
			err,
		)
	}
	if !isActiveMember(member) {
		return Conversation{}, ConversationMember{}, ErrForbidden
	}

	return conversationRow, member, nil
}

func (s *Service) appendConversationEvent(
	ctx context.Context,
	store Store,
	conversationRow Conversation,
	actorAccountID string,
	actorDeviceID string,
	eventType EventType,
	payloadType string,
	metadata map[string]string,
	createdAt time.Time,
) (Conversation, EventEnvelope, error) {
	if createdAt.IsZero() {
		createdAt = s.currentTime()
	}

	event, err := store.SaveEvent(ctx, EventEnvelope{
		EventID:        eventID(),
		EventType:      eventType,
		ConversationID: conversationRow.ID,
		ActorAccountID: strings.TrimSpace(actorAccountID),
		ActorDeviceID:  strings.TrimSpace(actorDeviceID),
		PayloadType:    strings.TrimSpace(payloadType),
		Metadata:       trimMetadata(metadata),
		CreatedAt:      createdAt,
	})
	if err != nil {
		return Conversation{}, EventEnvelope{}, fmt.Errorf(
			"save conversation event %s for %s: %w",
			eventType,
			conversationRow.ID,
			err,
		)
	}

	conversationRow.LastSequence = event.Sequence
	conversationRow.UpdatedAt = createdAt
	savedConversation, err := store.SaveConversation(ctx, conversationRow)
	if err != nil {
		return Conversation{}, EventEnvelope{}, fmt.Errorf("update conversation sequence %s: %w", conversationRow.ID, err)
	}

	return savedConversation, event, nil
}

// PublishConversationUpdate appends a sync-visible conversation.updated event without mutating settings or members.
func (s *Service) PublishConversationUpdate(
	ctx context.Context,
	params PublishConversationUpdateParams,
) (Conversation, EventEnvelope, error) {
	if err := s.validateContext(ctx, "publish conversation update"); err != nil {
		return Conversation{}, EventEnvelope{}, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	params.DeviceID = strings.TrimSpace(params.DeviceID)
	params.PayloadType = strings.TrimSpace(params.PayloadType)
	if params.ConversationID == "" || params.AccountID == "" || params.PayloadType == "" {
		return Conversation{}, EventEnvelope{}, ErrInvalidInput
	}

	var (
		saved Conversation
		event EventEnvelope
	)
	err := s.store.WithinTx(ctx, func(tx Store) error {
		conversationRow, _, err := s.loadConversationActor(ctx, tx, params.ConversationID, params.AccountID)
		if err != nil {
			return err
		}

		saved, event, err = s.appendConversationEvent(
			ctx,
			tx,
			conversationRow,
			params.AccountID,
			params.DeviceID,
			EventTypeConversationUpdated,
			params.PayloadType,
			params.Metadata,
			params.CreatedAt,
		)
		return err
	})
	if err != nil {
		return Conversation{}, EventEnvelope{}, err
	}

	return saved, event, nil
}

func canManageConversation(member ConversationMember, actorAccountID string, kind ConversationKind) bool {
	if member.AccountID != actorAccountID || !isActiveMember(member) {
		return false
	}

	switch kind {
	case ConversationKindDirect, ConversationKindSavedMessages:
		return member.Role == MemberRoleOwner
	default:
		return member.Role == MemberRoleOwner || member.Role == MemberRoleAdmin
	}
}
