package conversation

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

const generalTopicTitle = "General"

// CreateTopic creates a new topic inside a threaded conversation.
func (s *Service) CreateTopic(ctx context.Context, params CreateTopicParams) (ConversationTopic, EventEnvelope, error) {
	if err := s.validateContext(ctx, "create topic"); err != nil {
		return ConversationTopic{}, EventEnvelope{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.CreatorAccountID = strings.TrimSpace(params.CreatorAccountID)
	params.Title = strings.TrimSpace(params.Title)
	if params.ConversationID == "" || params.CreatorAccountID == "" || params.Title == "" {
		return ConversationTopic{}, EventEnvelope{}, ErrInvalidInput
	}

	now := params.CreatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedTopic ConversationTopic
		savedEvent EventEnvelope
	)

	err := s.runTx(ctx, func(tx Store) error {
		conversation, err := tx.ConversationByID(ctx, params.ConversationID)
		if err != nil {
			return fmt.Errorf("load conversation %s: %w", params.ConversationID, err)
		}
		policy, err := s.policyForConversation(ctx, tx, conversation, "")
		if err != nil {
			return err
		}
		if conversation.Kind != ConversationKindGroup || !policy.AllowThreads {
			return ErrForbidden
		}

		member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.CreatorAccountID)
		if err != nil {
			return fmt.Errorf("authorize topic creator %s in conversation %s: %w", params.CreatorAccountID, conversation.ID, err)
		}
		if !isActiveMember(member) || !canManageTopics(member) {
			return ErrForbidden
		}

		topicID, idErr := newID("top")
		if idErr != nil {
			return fmt.Errorf("generate topic id: %w", idErr)
		}

		topic := ConversationTopic{
			ConversationID:     conversation.ID,
			ID:                 topicID,
			Title:              params.Title,
			CreatedByAccountID: params.CreatorAccountID,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		savedTopic, err = tx.SaveTopic(ctx, topic)
		if err != nil {
			return fmt.Errorf("save topic %s: %w", topic.ID, err)
		}

		savedEvent, err = tx.SaveEvent(ctx, EventEnvelope{
			EventID:        eventID(),
			EventType:      EventTypeTopicCreated,
			ConversationID: conversation.ID,
			ActorAccountID: params.CreatorAccountID,
			Metadata: topicEventMetadata(savedTopic.ID, map[string]string{
				"title": savedTopic.Title,
			}),
			CreatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("save topic created event: %w", err)
		}

		savedTopic.LastSequence = savedEvent.Sequence
		savedTopic.UpdatedAt = now
		savedTopic, err = tx.SaveTopic(ctx, savedTopic)
		if err != nil {
			return fmt.Errorf("update topic %s sequence: %w", savedTopic.ID, err)
		}

		conversation.LastSequence = savedEvent.Sequence
		conversation.UpdatedAt = now
		if _, err := tx.SaveConversation(ctx, conversation); err != nil {
			return fmt.Errorf("update conversation %s after topic create: %w", conversation.ID, err)
		}

		return nil
	})
	if err != nil {
		return ConversationTopic{}, EventEnvelope{}, err
	}

	s.indexTopic(ctx, savedTopic)
	s.indexConversationByID(ctx, savedTopic.ConversationID)

	return savedTopic, savedEvent, nil
}

// GetTopic loads a topic together with access control.
func (s *Service) GetTopic(ctx context.Context, params GetTopicParams) (ConversationTopic, error) {
	if err := s.validateContext(ctx, "get topic"); err != nil {
		return ConversationTopic{}, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.TopicID = strings.TrimSpace(params.TopicID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.ConversationID == "" || params.AccountID == "" {
		return ConversationTopic{}, ErrInvalidInput
	}

	conversation, err := s.store.ConversationByID(ctx, params.ConversationID)
	if err != nil {
		return ConversationTopic{}, fmt.Errorf("load conversation %s: %w", params.ConversationID, err)
	}
	if conversation.Kind != ConversationKindGroup {
		return ConversationTopic{}, ErrForbidden
	}

	member, err := s.store.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.AccountID)
	if err != nil {
		return ConversationTopic{}, fmt.Errorf("authorize conversation %s for account %s: %w", conversation.ID, params.AccountID, err)
	}
	if !isActiveMember(member) {
		return ConversationTopic{}, ErrForbidden
	}

	policy, err := s.policyForConversation(ctx, s.store, conversation, params.TopicID)
	if err != nil {
		return ConversationTopic{}, err
	}
	if !policy.AllowThreads {
		return ConversationTopic{}, ErrForbidden
	}

	topic, err := s.store.TopicByConversationAndID(ctx, conversation.ID, params.TopicID)
	if err != nil {
		return ConversationTopic{}, fmt.Errorf("load topic %s in conversation %s: %w", params.TopicID, conversation.ID, err)
	}

	return topic, nil
}

// ListTopics returns the topics visible to one conversation member.
func (s *Service) ListTopics(ctx context.Context, params ListTopicsParams) ([]ConversationTopic, error) {
	if err := s.validateContext(ctx, "list topics"); err != nil {
		return nil, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.ConversationID == "" || params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	conversation, err := s.store.ConversationByID(ctx, params.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("load conversation %s: %w", params.ConversationID, err)
	}
	if conversation.Kind != ConversationKindGroup {
		return nil, ErrForbidden
	}

	member, err := s.store.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.AccountID)
	if err != nil {
		return nil, fmt.Errorf("authorize conversation %s for account %s: %w", conversation.ID, params.AccountID, err)
	}
	if !isActiveMember(member) {
		return nil, ErrForbidden
	}

	policy, err := s.policyForConversation(ctx, s.store, conversation, "")
	if err != nil {
		return nil, err
	}
	if !policy.AllowThreads {
		return nil, ErrForbidden
	}

	topics, err := s.store.TopicsByConversationID(ctx, conversation.ID)
	if err != nil {
		return nil, fmt.Errorf("load topics for conversation %s: %w", conversation.ID, err)
	}

	filtered := make([]ConversationTopic, 0, len(topics))
	for _, topic := range topics {
		if !params.IncludeArchived && topic.Archived {
			continue
		}
		if !params.IncludeClosed && topic.Closed {
			continue
		}
		filtered = append(filtered, topic)
	}

	slices.SortFunc(filtered, func(a, b ConversationTopic) int {
		switch {
		case a.IsGeneral && !b.IsGeneral:
			return -1
		case !a.IsGeneral && b.IsGeneral:
			return 1
		case a.Pinned && !b.Pinned:
			return -1
		case !a.Pinned && b.Pinned:
			return 1
		case a.LastSequence > b.LastSequence:
			return -1
		case a.LastSequence < b.LastSequence:
			return 1
		case a.UpdatedAt.After(b.UpdatedAt):
			return -1
		case a.UpdatedAt.Before(b.UpdatedAt):
			return 1
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})

	return filtered, nil
}

// RenameTopic renames an existing topic.
func (s *Service) RenameTopic(ctx context.Context, params RenameTopicParams) (ConversationTopic, EventEnvelope, error) {
	return s.updateTopic(ctx, "rename topic", params.ConversationID, params.TopicID, params.ActorAccountID, params.UpdatedAt, func(topic ConversationTopic, now time.Time) (ConversationTopic, EventType, map[string]string, error) {
		if topic.IsGeneral {
			return ConversationTopic{}, EventTypeUnspecified, nil, ErrConflict
		}
		params.Title = strings.TrimSpace(params.Title)
		if params.Title == "" {
			return ConversationTopic{}, EventTypeUnspecified, nil, ErrInvalidInput
		}
		if topic.Title == params.Title {
			return topic, EventTypeUnspecified, nil, nil
		}

		topic.Title = params.Title
		topic.UpdatedAt = now
		return topic, EventTypeTopicUpdated, map[string]string{
			"topic_id": topic.ID,
			"title":    topic.Title,
		}, nil
	})
}

// ArchiveTopic archives or unarchives a topic.
func (s *Service) ArchiveTopic(ctx context.Context, params ArchiveTopicParams) (ConversationTopic, EventEnvelope, error) {
	return s.updateTopic(ctx, "archive topic", params.ConversationID, params.TopicID, params.ActorAccountID, params.UpdatedAt, func(topic ConversationTopic, now time.Time) (ConversationTopic, EventType, map[string]string, error) {
		if topic.IsGeneral {
			return ConversationTopic{}, EventTypeUnspecified, nil, ErrConflict
		}
		if topic.Archived == params.Archived {
			return topic, EventTypeUnspecified, nil, nil
		}

		topic.Archived = params.Archived
		topic.UpdatedAt = now
		if params.Archived {
			topic.ArchivedAt = now
		} else {
			topic.ArchivedAt = time.Time{}
		}
		eventType := EventTypeTopicArchived
		if !params.Archived {
			eventType = EventTypeTopicUpdated
		}
		return topic, eventType, map[string]string{
			"topic_id": topic.ID,
			"archived": fmt.Sprintf("%t", topic.Archived),
		}, nil
	})
}

// CloseTopic closes or reopens a topic.
func (s *Service) CloseTopic(ctx context.Context, params CloseTopicParams) (ConversationTopic, EventEnvelope, error) {
	return s.updateTopic(ctx, "close topic", params.ConversationID, params.TopicID, params.ActorAccountID, params.UpdatedAt, func(topic ConversationTopic, now time.Time) (ConversationTopic, EventType, map[string]string, error) {
		if topic.IsGeneral {
			return ConversationTopic{}, EventTypeUnspecified, nil, ErrConflict
		}
		if topic.Closed == params.Closed {
			return topic, EventTypeUnspecified, nil, nil
		}

		topic.Closed = params.Closed
		topic.UpdatedAt = now
		if params.Closed {
			topic.ClosedAt = now
		} else {
			topic.ClosedAt = time.Time{}
		}
		eventType := EventTypeTopicClosed
		if !params.Closed {
			eventType = EventTypeTopicUpdated
		}
		return topic, eventType, map[string]string{
			"topic_id": topic.ID,
			"closed":   fmt.Sprintf("%t", topic.Closed),
		}, nil
	})
}

// PinTopic pins or unpins a topic.
func (s *Service) PinTopic(ctx context.Context, params PinTopicParams) (ConversationTopic, EventEnvelope, error) {
	return s.updateTopic(ctx, "pin topic", params.ConversationID, params.TopicID, params.ActorAccountID, params.UpdatedAt, func(topic ConversationTopic, now time.Time) (ConversationTopic, EventType, map[string]string, error) {
		if topic.IsGeneral {
			return ConversationTopic{}, EventTypeUnspecified, nil, ErrConflict
		}
		if topic.Pinned == params.Pinned {
			return topic, EventTypeUnspecified, nil, nil
		}

		topic.Pinned = params.Pinned
		topic.UpdatedAt = now
		return topic, EventTypeTopicPinned, map[string]string{
			"topic_id": topic.ID,
			"pinned":   fmt.Sprintf("%t", topic.Pinned),
		}, nil
	})
}

func (s *Service) updateTopic(
	ctx context.Context,
	operation string,
	conversationID string,
	topicID string,
	actorAccountID string,
	updatedAt time.Time,
	apply func(ConversationTopic, time.Time) (ConversationTopic, EventType, map[string]string, error),
) (ConversationTopic, EventEnvelope, error) {
	if err := s.validateContext(ctx, operation); err != nil {
		return ConversationTopic{}, EventEnvelope{}, err
	}
	conversationID = strings.TrimSpace(conversationID)
	topicID = strings.TrimSpace(topicID)
	actorAccountID = strings.TrimSpace(actorAccountID)
	if conversationID == "" || actorAccountID == "" {
		return ConversationTopic{}, EventEnvelope{}, ErrInvalidInput
	}

	now := updatedAt
	if now.IsZero() {
		now = s.currentTime()
	}

	var (
		savedTopic ConversationTopic
		savedEvent EventEnvelope
	)

	err := s.runTx(ctx, func(tx Store) error {
		conversation, err := tx.ConversationByID(ctx, conversationID)
		if err != nil {
			return fmt.Errorf("load conversation %s: %w", conversationID, err)
		}
		if conversation.Kind != ConversationKindGroup {
			return ErrForbidden
		}

		policy, err := s.policyForConversation(ctx, tx, conversation, topicID)
		if err != nil {
			return err
		}
		if !policy.AllowThreads {
			return ErrForbidden
		}

		member, err := tx.ConversationMemberByConversationAndAccount(ctx, conversation.ID, actorAccountID)
		if err != nil {
			return fmt.Errorf("authorize topic actor %s in conversation %s: %w", actorAccountID, conversation.ID, err)
		}
		if !isActiveMember(member) || !canManageTopics(member) {
			return ErrForbidden
		}

		topic, err := tx.TopicByConversationAndID(ctx, conversation.ID, topicID)
		if err != nil {
			return fmt.Errorf("load topic %s in conversation %s: %w", topicID, conversation.ID, err)
		}

		nextTopic, eventType, metadata, applyErr := apply(topic, now)
		if applyErr != nil {
			return applyErr
		}
		if eventType == EventTypeUnspecified {
			savedTopic = nextTopic
			return nil
		}

		savedTopic, err = tx.SaveTopic(ctx, nextTopic)
		if err != nil {
			return fmt.Errorf("save topic %s: %w", nextTopic.ID, err)
		}

		savedEvent, err = tx.SaveEvent(ctx, EventEnvelope{
			EventID:        eventID(),
			EventType:      eventType,
			ConversationID: conversation.ID,
			ActorAccountID: actorAccountID,
			Metadata:       topicEventMetadata(savedTopic.ID, metadata),
			CreatedAt:      now,
		})
		if err != nil {
			return fmt.Errorf("save topic event: %w", err)
		}

		savedTopic.LastSequence = savedEvent.Sequence
		savedTopic.UpdatedAt = now
		savedTopic, err = tx.SaveTopic(ctx, savedTopic)
		if err != nil {
			return fmt.Errorf("update topic %s sequence: %w", savedTopic.ID, err)
		}

		conversation.LastSequence = savedEvent.Sequence
		conversation.UpdatedAt = now
		if _, err := tx.SaveConversation(ctx, conversation); err != nil {
			return fmt.Errorf("update conversation %s after topic update: %w", conversation.ID, err)
		}

		return nil
	})
	if err != nil {
		return ConversationTopic{}, EventEnvelope{}, err
	}

	s.indexTopic(ctx, savedTopic)
	s.indexConversationByID(ctx, savedTopic.ConversationID)

	return savedTopic, savedEvent, nil
}

func canManageTopics(member ConversationMember) bool {
	return member.Role == MemberRoleOwner || member.Role == MemberRoleAdmin
}

func topicEventMetadata(topicID string, extra map[string]string) map[string]string {
	metadata := trimMetadata(extra)
	if topicID != "" {
		if metadata == nil {
			metadata = make(map[string]string, 1)
		}
		metadata["topic_id"] = topicID
	}

	return metadata
}
