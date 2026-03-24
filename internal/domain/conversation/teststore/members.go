package teststore

import (
	"context"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func (s *memoryStore) SaveConversationMember(ctx context.Context, member conversation.ConversationMember) (conversation.ConversationMember, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ConversationMember{}, err
	}
	member.ConversationID = strings.TrimSpace(member.ConversationID)
	member.AccountID = strings.TrimSpace(member.AccountID)
	if member.ConversationID == "" || member.AccountID == "" {
		return conversation.ConversationMember{}, conversation.ErrInvalidInput
	}

	key := conversationMemberKey(member.ConversationID, member.AccountID)
	s.membersByKey[key] = cloneMember(member)
	return cloneMember(member), nil
}

func (s *memoryStore) ConversationMemberByConversationAndAccount(ctx context.Context, conversationID string, accountID string) (conversation.ConversationMember, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.ConversationMember{}, err
	}
	key := conversationMemberKey(strings.TrimSpace(conversationID), strings.TrimSpace(accountID))
	member, ok := s.membersByKey[key]
	if !ok {
		return conversation.ConversationMember{}, conversation.ErrNotFound
	}

	return cloneMember(member), nil
}

func (s *memoryStore) ConversationMembersByConversationID(ctx context.Context, conversationID string) ([]conversation.ConversationMember, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, conversation.ErrInvalidInput
	}

	members := make([]conversation.ConversationMember, 0)
	for _, member := range s.membersByKey {
		if member.ConversationID != conversationID {
			continue
		}
		members = append(members, cloneMember(member))
	}

	return members, nil
}
