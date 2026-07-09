package teststore

import (
	"context"
	"sort"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

func (s *memoryStore) SaveConversationInvite(ctx context.Context, invite conversation.ConversationInvite) (conversation.ConversationInvite, error) {
	if err := s.validateWrite(ctx); err != nil {
		return conversation.ConversationInvite{}, err
	}

	invite.ConversationID = strings.TrimSpace(invite.ConversationID)
	invite.ID = strings.TrimSpace(invite.ID)
	invite.Code = strings.TrimSpace(invite.Code)
	if invite.ConversationID == "" || invite.ID == "" || invite.Code == "" || len(invite.AllowedRoles) == 0 {
		return conversation.ConversationInvite{}, conversation.ErrInvalidInput
	}

	key := inviteKey(invite.ConversationID, invite.ID)
	s.invitesByKey[key] = cloneInvite(invite)
	return cloneInvite(invite), nil
}

func (s *memoryStore) ConversationInviteByConversationAndID(
	ctx context.Context,
	conversationID string,
	inviteID string,
) (conversation.ConversationInvite, error) {
	if err := s.validateRead(ctx); err != nil {
		return conversation.ConversationInvite{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	invite, ok := s.invitesByKey[inviteKey(strings.TrimSpace(conversationID), strings.TrimSpace(inviteID))]
	if !ok {
		return conversation.ConversationInvite{}, conversation.ErrNotFound
	}

	return cloneInvite(invite), nil
}

func (s *memoryStore) ConversationInvitesByConversationID(
	ctx context.Context,
	conversationID string,
) ([]conversation.ConversationInvite, error) {
	if err := s.validateRead(ctx); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, conversation.ErrInvalidInput
	}

	invites := make([]conversation.ConversationInvite, 0)
	for _, invite := range s.invitesByKey {
		if invite.ConversationID != conversationID {
			continue
		}
		invites = append(invites, cloneInvite(invite))
	}

	sort.Slice(invites, func(i, j int) bool {
		if invites[i].CreatedAt.Equal(invites[j].CreatedAt) {
			return invites[i].ID < invites[j].ID
		}
		return invites[i].CreatedAt.After(invites[j].CreatedAt)
	})
	return invites, nil
}
