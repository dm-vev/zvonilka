package conversation

import (
	"context"
	"fmt"
	"strings"
)

// ListMembers returns the members of a conversation visible to the caller.
func (s *Service) ListMembers(ctx context.Context, params GetConversationParams) ([]ConversationMember, error) {
	if err := s.validateContext(ctx, "list conversation members"); err != nil {
		return nil, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.ConversationID == "" || params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	member, err := s.store.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.AccountID)
	if err != nil {
		return nil, fmt.Errorf(
			"authorize member list for conversation %s and account %s: %w",
			params.ConversationID,
			params.AccountID,
			err,
		)
	}
	if !isActiveMember(member) {
		return nil, ErrForbidden
	}

	members, err := s.store.ConversationMembersByConversationID(ctx, params.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("list members for conversation %s: %w", params.ConversationID, err)
	}

	return members, nil
}

// GetMessage resolves one conversation message visible to the caller.
func (s *Service) GetMessage(ctx context.Context, params GetMessageParams) (Message, error) {
	if err := s.validateContext(ctx, "get message"); err != nil {
		return Message{}, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.ConversationID == "" || params.MessageID == "" || params.AccountID == "" {
		return Message{}, ErrInvalidInput
	}

	member, err := s.store.ConversationMemberByConversationAndAccount(ctx, params.ConversationID, params.AccountID)
	if err != nil {
		return Message{}, fmt.Errorf(
			"authorize message read for conversation %s and account %s: %w",
			params.ConversationID,
			params.AccountID,
			err,
		)
	}
	if !isActiveMember(member) {
		return Message{}, ErrForbidden
	}

	message, err := s.store.MessageByID(ctx, params.ConversationID, params.MessageID)
	if err != nil {
		return Message{}, fmt.Errorf(
			"load message %s in conversation %s: %w",
			params.MessageID,
			params.ConversationID,
			err,
		)
	}
	if (message.Status == MessageStatusPending || message.Status == MessageStatusFailed) &&
		message.SenderAccountID != params.AccountID {
		return Message{}, ErrNotFound
	}
	if !message.DeletedAt.IsZero() && member.Role != MemberRoleOwner && member.Role != MemberRoleAdmin {
		return Message{}, ErrNotFound
	}

	return message, nil
}
