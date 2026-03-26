package teststore

import (
	"context"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/storage"
)

// WithinTx executes the callback against a transaction-like snapshot.
func (s *memoryStore) WithinTx(ctx context.Context, fn func(conversation.Store) error) error {
	if ctx == nil {
		return conversation.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if fn == nil {
		return conversation.ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx := s.cloneLocked()
	err := fn(&tx)
	if err == nil {
		s.replaceLocked(&tx)
		return nil
	}
	if storage.IsCommit(err) {
		s.replaceLocked(&tx)
		return storage.UnwrapCommit(err)
	}

	return err
}

func (s *memoryStore) cloneLocked() memoryStore {
	return memoryStore{
		conversationsByID:       cloneConversations(s.conversationsByID),
		topicsByKey:             cloneTopics(s.topicsByKey),
		membersByKey:            cloneMembers(s.membersByKey),
		invitesByKey:            cloneInvites(s.invitesByKey),
		messagesByID:            cloneMessages(s.messagesByID),
		reactionsByKey:          cloneReactions(s.reactionsByKey),
		readStatesByKey:         cloneReadStates(s.readStatesByKey),
		syncStatesByDevice:      cloneSyncStates(s.syncStatesByDevice),
		eventsByID:              cloneEvents(s.eventsByID),
		moderationPoliciesByKey: cloneModerationPolicies(s.moderationPoliciesByKey),
		moderationReportsByID:   cloneModerationReports(s.moderationReportsByID),
		moderationActionsByID:   cloneModerationActions(s.moderationActionsByID),
		moderationRestrictions:  cloneModerationRestrictions(s.moderationRestrictions),
		moderationRateStates:    cloneModerationRateStates(s.moderationRateStates),
		eventOrder:              append([]string(nil), s.eventOrder...),
		nextSequence:            s.nextSequence,
	}
}

func (s *memoryStore) replaceLocked(tx *memoryStore) {
	s.conversationsByID = cloneConversations(tx.conversationsByID)
	s.topicsByKey = cloneTopics(tx.topicsByKey)
	s.membersByKey = cloneMembers(tx.membersByKey)
	s.invitesByKey = cloneInvites(tx.invitesByKey)
	s.messagesByID = cloneMessages(tx.messagesByID)
	s.reactionsByKey = cloneReactions(tx.reactionsByKey)
	s.readStatesByKey = cloneReadStates(tx.readStatesByKey)
	s.syncStatesByDevice = cloneSyncStates(tx.syncStatesByDevice)
	s.eventsByID = cloneEvents(tx.eventsByID)
	s.moderationPoliciesByKey = cloneModerationPolicies(tx.moderationPoliciesByKey)
	s.moderationReportsByID = cloneModerationReports(tx.moderationReportsByID)
	s.moderationActionsByID = cloneModerationActions(tx.moderationActionsByID)
	s.moderationRestrictions = cloneModerationRestrictions(tx.moderationRestrictions)
	s.moderationRateStates = cloneModerationRateStates(tx.moderationRateStates)
	s.eventOrder = append([]string(nil), tx.eventOrder...)
	s.nextSequence = tx.nextSequence
}
