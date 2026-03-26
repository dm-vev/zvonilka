package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func findMessageReaction(reactions []MessageReaction, accountID string) (MessageReaction, bool) {
	for _, reaction := range reactions {
		if reaction.AccountID == accountID {
			return reaction, true
		}
	}

	return MessageReaction{}, false
}

func (s *Service) finalizeMessageReaction(
	ctx context.Context,
	tx Store,
	state messageActionState,
	now time.Time,
	event EventEnvelope,
) error {
	if err := s.finalizeMessageAction(ctx, tx, state, now, event); err != nil {
		return err
	}

	return nil
}

// AddMessageReaction creates or updates a reaction for a message.
func (s *Service) AddMessageReaction(ctx context.Context, params AddMessageReactionParams) (Message, EventEnvelope, error) {
	if err := s.validateContext(ctx, "add message reaction"); err != nil {
		return Message{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.ActorDeviceID = strings.TrimSpace(params.ActorDeviceID)
	params.Reaction = strings.TrimSpace(params.Reaction)
	if params.ConversationID == "" || params.MessageID == "" || params.ActorAccountID == "" || params.Reaction == "" {
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
		state, err := s.loadMessageActionState(ctx, tx, "load message for reaction", params.ConversationID, params.MessageID, params.ActorAccountID)
		if err != nil {
			return err
		}
		policy, err := s.policyForConversation(ctx, tx, state.conversation, state.topic.ID)
		if err != nil {
			return err
		}
		if !policy.AllowReactions {
			return ErrForbidden
		}
		checkPolicy := policy
		checkPolicy.SlowModeInterval = 0
		checkPolicy.AntiSpamWindow = 0
		checkPolicy.AntiSpamBurstLimit = 0
		targetKind, targetID := moderationTarget(state.conversation, state.topic.ID)
		decision, err := s.checkModerationWrite(ctx, tx, CheckModerationWriteParams{
			TargetKind:     targetKind,
			TargetID:       targetID,
			ActorAccountID: params.ActorAccountID,
			ActorRole:      state.member.Role,
			BasePolicy:     checkPolicy,
			CreatedAt:      now,
		})
		if err != nil {
			return err
		}
		if !decision.Allowed {
			return ErrForbidden
		}
		if !state.message.DeletedAt.IsZero() || state.message.Status == MessageStatusDeleted {
			return ErrConflict
		}

		existingReaction, found := findMessageReaction(state.message.Reactions, params.ActorAccountID)
		if found && existingReaction.Reaction == params.Reaction {
			savedMessage = state.message
			return nil
		}

		reaction := MessageReaction{
			MessageID: state.message.ID,
			AccountID: params.ActorAccountID,
			Reaction:  params.Reaction,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if found {
			reaction.CreatedAt = existingReaction.CreatedAt
		}

		savedReaction, saveErr := tx.SaveMessageReaction(ctx, reaction)
		if saveErr != nil {
			return fmt.Errorf("save message reaction %s/%s: %w", reaction.MessageID, reaction.AccountID, saveErr)
		}

		eventType := EventTypeMessageReactionAdded
		if found {
			eventType = EventTypeMessageReactionUpdated
		}

		savedEvent, err = tx.SaveEvent(ctx, EventEnvelope{
			EventID:        eventID(),
			EventType:      eventType,
			ConversationID: state.conversation.ID,
			ActorAccountID: params.ActorAccountID,
			ActorDeviceID:  params.ActorDeviceID,
			MessageID:      state.message.ID,
			PayloadType:    "message",
			Metadata: messageEventMetadata(state.message.ID, state.message.ThreadID, state.message.ReplyTo, map[string]string{
				"action":   "reaction",
				"reaction": savedReaction.Reaction,
			}),
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("save message reaction event: %w", err)
		}

		if err := s.finalizeMessageReaction(ctx, tx, state, now, savedEvent); err != nil {
			return err
		}

		savedMessage, err = tx.MessageByID(ctx, state.conversation.ID, state.message.ID)
		if err != nil {
			return fmt.Errorf("reload reacted message %s: %w", state.message.ID, err)
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

// RemoveMessageReaction removes a reaction from a message.
func (s *Service) RemoveMessageReaction(ctx context.Context, params RemoveMessageReactionParams) (Message, EventEnvelope, error) {
	if err := s.validateContext(ctx, "remove message reaction"); err != nil {
		return Message{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	params.ActorAccountID = strings.TrimSpace(params.ActorAccountID)
	params.ActorDeviceID = strings.TrimSpace(params.ActorDeviceID)
	params.Reaction = strings.TrimSpace(params.Reaction)
	if params.ConversationID == "" || params.MessageID == "" || params.ActorAccountID == "" {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}

	now := params.RemovedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedMessage Message
		savedEvent   EventEnvelope
	)

	err := s.runTx(ctx, func(tx Store) error {
		state, err := s.loadMessageActionState(ctx, tx, "load message for reaction removal", params.ConversationID, params.MessageID, params.ActorAccountID)
		if err != nil {
			return err
		}
		policy, err := s.policyForConversation(ctx, tx, state.conversation, state.topic.ID)
		if err != nil {
			return err
		}
		if !policy.AllowReactions {
			return ErrForbidden
		}
		checkPolicy := policy
		checkPolicy.SlowModeInterval = 0
		checkPolicy.AntiSpamWindow = 0
		checkPolicy.AntiSpamBurstLimit = 0
		targetKind, targetID := moderationTarget(state.conversation, state.topic.ID)
		decision, err := s.checkModerationWrite(ctx, tx, CheckModerationWriteParams{
			TargetKind:     targetKind,
			TargetID:       targetID,
			ActorAccountID: params.ActorAccountID,
			ActorRole:      state.member.Role,
			BasePolicy:     checkPolicy,
			CreatedAt:      now,
		})
		if err != nil {
			return err
		}
		if !decision.Allowed {
			return ErrForbidden
		}
		if !state.message.DeletedAt.IsZero() || state.message.Status == MessageStatusDeleted {
			return ErrConflict
		}

		existingReaction, found := findMessageReaction(state.message.Reactions, params.ActorAccountID)
		if !found {
			savedMessage = state.message
			return nil
		}
		if params.Reaction != "" && existingReaction.Reaction != params.Reaction {
			return ErrConflict
		}

		if err := tx.DeleteMessageReaction(ctx, state.message.ID, params.ActorAccountID); err != nil {
			return fmt.Errorf("delete message reaction %s/%s: %w", state.message.ID, params.ActorAccountID, err)
		}

		savedEvent, err = tx.SaveEvent(ctx, EventEnvelope{
			EventID:        eventID(),
			EventType:      EventTypeMessageReactionRemoved,
			ConversationID: state.conversation.ID,
			ActorAccountID: params.ActorAccountID,
			ActorDeviceID:  params.ActorDeviceID,
			MessageID:      state.message.ID,
			PayloadType:    "message",
			Metadata: messageEventMetadata(state.message.ID, state.message.ThreadID, state.message.ReplyTo, map[string]string{
				"action":   "reaction",
				"reaction": existingReaction.Reaction,
			}),
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("save message reaction removal event: %w", err)
		}

		if err := s.finalizeMessageReaction(ctx, tx, state, now, savedEvent); err != nil {
			return err
		}

		savedMessage, err = tx.MessageByID(ctx, state.conversation.ID, state.message.ID)
		if err != nil {
			return fmt.Errorf("reload reacted message %s: %w", state.message.ID, err)
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
