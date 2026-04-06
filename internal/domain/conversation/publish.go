package conversation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type publishMessageState struct {
	Conversation Conversation
	Topic        ConversationTopic
	Policy       ModerationPolicy
}

type publishMessageParams struct {
	Message        Message
	State          publishMessageState
	Decision       ModerationDecision
	ActorAccountID string
	ActorDeviceID  string
	CausationID    string
	CorrelationID  string
	PublishedAt    time.Time
}

func (s *Service) validateMessageForPublication(
	ctx context.Context,
	tx Store,
	message Message,
	publishedAt time.Time,
) (publishMessageState, ModerationDecision, error) {
	conversationRow, err := tx.ConversationByID(ctx, message.ConversationID)
	if err != nil {
		return publishMessageState{}, ModerationDecision{}, fmt.Errorf("load conversation %s: %w", message.ConversationID, err)
	}

	member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversationRow.ID, message.SenderAccountID)
	if err != nil {
		return publishMessageState{}, ModerationDecision{}, fmt.Errorf(
			"authorize sender %s for conversation %s: %w",
			message.SenderAccountID,
			conversationRow.ID,
			err,
		)
	}
	if !isActiveMember(member) {
		return publishMessageState{}, ModerationDecision{}, ErrForbidden
	}

	topic, err := tx.TopicByConversationAndID(ctx, conversationRow.ID, message.ThreadID)
	if err != nil {
		return publishMessageState{}, ModerationDecision{}, fmt.Errorf(
			"load topic %s in conversation %s: %w",
			message.ThreadID,
			conversationRow.ID,
			err,
		)
	}
	if topic.Archived || topic.Closed {
		return publishMessageState{}, ModerationDecision{}, ErrForbidden
	}

	policy, err := s.policyForConversation(ctx, tx, conversationRow, topic.ID)
	if err != nil {
		return publishMessageState{}, ModerationDecision{}, err
	}
	if message.ThreadID != "" && !policy.AllowThreads {
		return publishMessageState{}, ModerationDecision{}, ErrForbidden
	}
	if err := ValidateMessagePayload(message.Payload, policy.RequireEncryptedMessages); err != nil {
		return publishMessageState{}, ModerationDecision{}, ErrInvalidInput
	}

	if message.ReplyTo.MessageID != "" {
		replyTarget, replyErr := tx.MessageByID(ctx, conversationRow.ID, message.ReplyTo.MessageID)
		if replyErr != nil {
			return publishMessageState{}, ModerationDecision{}, fmt.Errorf(
				"load reply target %s in conversation %s: %w",
				message.ReplyTo.MessageID,
				conversationRow.ID,
				replyErr,
			)
		}
		if !replyTarget.DeletedAt.IsZero() || replyTarget.Status == MessageStatusDeleted {
			return publishMessageState{}, ModerationDecision{}, ErrConflict
		}
		if replyTarget.ThreadID != topic.ID {
			return publishMessageState{}, ModerationDecision{}, ErrConflict
		}
	}

	targetKind, targetID := moderationTarget(conversationRow, topic.ID)
	decision, err := s.checkModerationWrite(ctx, tx, CheckModerationWriteParams{
		TargetKind:     targetKind,
		TargetID:       targetID,
		ActorAccountID: message.SenderAccountID,
		ActorRole:      member.Role,
		BasePolicy:     policy,
		CreatedAt:      publishedAt,
	})
	if err != nil {
		return publishMessageState{}, decision, err
	}

	return publishMessageState{
		Conversation: conversationRow,
		Topic:        topic,
		Policy:       policy,
	}, decision, nil
}

func (s *Service) publishMessage(
	ctx context.Context,
	tx Store,
	params publishMessageParams,
) (Message, EventEnvelope, error) {
	message := params.Message
	now := params.PublishedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	message.Status = MessageStatusSent
	message.Sequence = 0
	message.CreatedAt = now
	message.UpdatedAt = now

	savedMessage, err := tx.SaveMessage(ctx, message)
	if err != nil {
		return Message{}, EventEnvelope{}, fmt.Errorf("save message: %w", err)
	}

	event := EventEnvelope{
		EventID:        eventID(),
		EventType:      EventTypeMessageCreated,
		ConversationID: params.State.Conversation.ID,
		ActorAccountID: params.ActorAccountID,
		ActorDeviceID:  params.ActorDeviceID,
		CausationID:    params.CausationID,
		CorrelationID:  params.CorrelationID,
		MessageID:      savedMessage.ID,
		PayloadType:    "message",
		Payload:        savedMessage.Payload,
		Metadata: messageEventMetadata(
			savedMessage.ID,
			savedMessage.ThreadID,
			savedMessage.ReplyTo,
			messageSnapshotMetadata(savedMessage, nil),
		),
		CreatedAt: now,
	}
	savedEvent, err := tx.SaveEvent(ctx, event)
	if err != nil {
		return Message{}, EventEnvelope{}, fmt.Errorf("save message event: %w", err)
	}

	targetKind, targetID := moderationTarget(params.State.Conversation, params.State.Topic.ID)
	rateState, stateErr := tx.ModerationRateStateByTargetAndAccount(ctx, targetKind, targetID, params.ActorAccountID)
	if stateErr != nil && !errors.Is(stateErr, ErrNotFound) {
		return Message{}, EventEnvelope{}, fmt.Errorf("load moderation rate state: %w", stateErr)
	}

	rateState.TargetKind = targetKind
	rateState.TargetID = targetID
	rateState.AccountID = params.ActorAccountID
	rateState.LastWriteAt = now
	if rateState.WindowStartedAt.IsZero() || now.Sub(rateState.WindowStartedAt) >= params.State.Policy.AntiSpamWindow {
		rateState.WindowStartedAt = now
		rateState.WindowCount = 1
	} else {
		rateState.WindowCount++
	}
	rateState.UpdatedAt = now
	if _, err := tx.SaveModerationRateState(ctx, rateState); err != nil {
		return Message{}, EventEnvelope{}, fmt.Errorf("save moderation rate state: %w", err)
	}

	params.State.Topic.LastSequence = savedEvent.Sequence
	params.State.Topic.MessageCount++
	params.State.Topic.LastMessageAt = now
	params.State.Topic.UpdatedAt = now
	if _, err := tx.SaveTopic(ctx, params.State.Topic); err != nil {
		return Message{}, EventEnvelope{}, fmt.Errorf("update topic %s after message: %w", params.State.Topic.ID, err)
	}

	savedMessage.Sequence = savedEvent.Sequence
	savedMessage.UpdatedAt = now
	savedMessage, err = tx.SaveMessage(ctx, savedMessage)
	if err != nil {
		return Message{}, EventEnvelope{}, fmt.Errorf("update message sequence: %w", err)
	}

	readState := ReadState{
		ConversationID:        params.State.Conversation.ID,
		AccountID:             params.ActorAccountID,
		DeviceID:              params.ActorDeviceID,
		LastDeliveredSequence: savedEvent.Sequence,
		LastReadSequence:      savedEvent.Sequence,
		LastAckedSequence:     savedEvent.Sequence,
		UpdatedAt:             now,
	}
	if _, err := tx.SaveReadState(ctx, readState); err != nil {
		return Message{}, EventEnvelope{}, fmt.Errorf("save sender read state: %w", err)
	}

	params.State.Conversation.LastSequence = savedEvent.Sequence
	params.State.Conversation.LastMessageAt = now
	params.State.Conversation.UpdatedAt = now
	if _, err := tx.SaveConversation(ctx, params.State.Conversation); err != nil {
		return Message{}, EventEnvelope{}, fmt.Errorf(
			"update conversation %s after message: %w",
			params.State.Conversation.ID,
			err,
		)
	}

	syncState := SyncState{
		DeviceID:            params.ActorDeviceID,
		AccountID:           params.ActorAccountID,
		LastAppliedSequence: savedEvent.Sequence,
		LastAckedSequence:   savedEvent.Sequence,
		ServerTime:          now,
	}
	if _, err := tx.SaveSyncState(ctx, syncState); err != nil {
		return Message{}, EventEnvelope{}, fmt.Errorf("save sync state: %w", err)
	}

	return savedMessage, savedEvent, nil
}

func scheduledFailureReason(err error) string {
	switch {
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrConflict):
		return "conflict"
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, ErrNotFound):
		return "not_found"
	default:
		return "internal_error"
	}
}

func (s *Service) markScheduledMessageFailed(
	ctx context.Context,
	tx Store,
	message Message,
	failedAt time.Time,
	reason string,
) (Message, error) {
	if failedAt.IsZero() {
		failedAt = s.currentTime()
	}

	metadata := trimMetadata(message.Metadata)
	if metadata == nil {
		metadata = make(map[string]string, 1)
	}
	if reason != "" {
		metadata["scheduled_failure_reason"] = reason
	}

	message.Status = MessageStatusFailed
	message.UpdatedAt = failedAt
	message.Metadata = metadata

	saved, err := tx.SaveMessage(ctx, message)
	if err != nil {
		return Message{}, fmt.Errorf("save failed scheduled message %s: %w", message.ID, err)
	}

	return saved, nil
}

func (s *Service) rescheduleScheduledMessage(
	ctx context.Context,
	tx Store,
	message Message,
	nextAttemptAt time.Time,
) (Message, error) {
	if nextAttemptAt.IsZero() {
		return message, nil
	}

	message.DeliverAt = nextAttemptAt.UTC()
	message.UpdatedAt = s.currentTime()

	saved, err := tx.SaveMessage(ctx, message)
	if err != nil {
		return Message{}, fmt.Errorf("reschedule message %s: %w", message.ID, err)
	}

	return saved, nil
}

// DispatchDueMessages publishes due scheduled messages and returns the produced sync events.
func (s *Service) DispatchDueMessages(ctx context.Context, before time.Time, limit int) ([]EventEnvelope, error) {
	if err := s.validateContext(ctx, "dispatch scheduled messages"); err != nil {
		return nil, err
	}

	if before.IsZero() {
		before = s.currentTime()
	}
	limit = clampLimit(limit, 100, 500)

	type dispatchedMessage struct {
		message      Message
		event        EventEnvelope
		shadowHidden bool
	}

	dispatched := make([]dispatchedMessage, 0, limit)
	err := s.runTx(ctx, func(tx Store) error {
		messages, err := tx.DueScheduledMessages(ctx, before, limit)
		if err != nil {
			return fmt.Errorf("load due scheduled messages: %w", err)
		}

		for _, message := range messages {
			state, decision, validateErr := s.validateMessageForPublication(ctx, tx, message, before)
			if validateErr != nil {
				if errors.Is(validateErr, ErrRateLimited) && decision.RetryAfter > 0 {
					if _, err := s.rescheduleScheduledMessage(ctx, tx, message, before.Add(decision.RetryAfter)); err != nil {
						return err
					}
					continue
				}

				if errors.Is(validateErr, ErrForbidden) ||
					errors.Is(validateErr, ErrConflict) ||
					errors.Is(validateErr, ErrInvalidInput) ||
					errors.Is(validateErr, ErrNotFound) {
					if _, err := s.markScheduledMessageFailed(
						ctx,
						tx,
						message,
						before,
						scheduledFailureReason(validateErr),
					); err != nil {
						return err
					}
					continue
				}

				return validateErr
			}

			savedMessage, event, err := s.publishMessage(ctx, tx, publishMessageParams{
				Message:        message,
				State:          state,
				Decision:       decision,
				ActorAccountID: message.SenderAccountID,
				ActorDeviceID:  message.SenderDeviceID,
				PublishedAt:    before,
			})
			if err != nil {
				return err
			}

			dispatched = append(dispatched, dispatchedMessage{
				message:      savedMessage,
				event:        event,
				shadowHidden: decision.ShadowHidden,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	events := make([]EventEnvelope, 0, len(dispatched))
	for _, item := range dispatched {
		if !item.shadowHidden {
			s.indexMessage(ctx, item.message)
		}
		s.indexConversationByID(ctx, item.message.ConversationID)
		events = append(events, item.event)
	}

	return events, nil
}
