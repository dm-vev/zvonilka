package conversation

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

// Service coordinates conversation, message, and sync state changes.
type Service struct {
	store   Store
	now     func() time.Time
	indexer domainsearch.Indexer
}

// NewService constructs a conversation service backed by the provided store.
func NewService(store Store, opts ...Option) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}

	service := &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}

	return s.now().UTC()
}

func (s *Service) validateContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fmt.Errorf("%s: %w", operation, ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return nil
}

func uniqueIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ids := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ids = append(ids, value)
	}

	return ids
}

func trimMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}

	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}

	return result
}

func maxUint64(values ...uint64) uint64 {
	var max uint64
	for _, value := range values {
		if value > max {
			max = value
		}
	}

	return max
}

func isActiveMember(member ConversationMember) bool {
	return member.LeftAt.IsZero() && !member.Banned
}

func conversationEventMetadata(messageID string, threadID string, extra map[string]string) map[string]string {
	metadata := trimMetadata(extra)
	if messageID != "" {
		if metadata == nil {
			metadata = make(map[string]string, 1)
		}
		metadata["message_id"] = messageID
	}
	if threadID != "" {
		if metadata == nil {
			metadata = make(map[string]string, 1)
		}
		metadata["thread_id"] = threadID
	}

	return metadata
}

func messageEventMetadata(
	messageID string,
	threadID string,
	replyTo MessageReference,
	extra map[string]string,
) map[string]string {
	metadata := conversationEventMetadata(messageID, threadID, extra)
	if replyTo.MessageID == "" {
		return metadata
	}
	if metadata == nil {
		metadata = make(map[string]string, 4)
	}
	metadata["reply_message_id"] = replyTo.MessageID
	if replyTo.ConversationID != "" {
		metadata["reply_conversation_id"] = replyTo.ConversationID
	}
	if replyTo.SenderAccountID != "" {
		metadata["reply_sender_account_id"] = replyTo.SenderAccountID
	}
	if replyTo.MessageKind != MessageKindUnspecified {
		metadata["reply_kind"] = string(replyTo.MessageKind)
	}

	return metadata
}

func messageSnapshotMetadata(message Message, extra map[string]string) map[string]string {
	metadata := trimMetadata(message.Metadata)
	if metadata == nil {
		metadata = make(map[string]string, 2)
	}
	metadata["message_kind"] = string(message.Kind)
	metadata["disable_link_previews"] = strconv.FormatBool(message.DisableLinkPreviews)

	for key, value := range trimMetadata(extra) {
		metadata[key] = value
	}

	return metadata
}

func ensureMemberIDs(ownerID string, memberIDs []string) ([]string, error) {
	ids := uniqueIDs(memberIDs)
	for _, memberID := range ids {
		if memberID == ownerID {
			return nil, ErrConflict
		}
	}

	return ids, nil
}

func clampLimit(limit int, defaultLimit int, maxLimit int) int {
	if limit <= 0 {
		limit = defaultLimit
	}
	if maxLimit > 0 && limit > maxLimit {
		return maxLimit
	}

	return limit
}

func (s *Service) runTx(ctx context.Context, fn func(Store) error) error {
	if err := s.validateContext(ctx, "conversation tx"); err != nil {
		return err
	}

	return s.store.WithinTx(ctx, fn)
}

func (s *Service) maybeCommit(err error) error {
	if errors.Is(err, ErrConflict) {
		return domainstorage.Commit(err)
	}

	return err
}

// CreateConversation creates a new conversation and its initial membership set.
func (s *Service) CreateConversation(ctx context.Context, params CreateConversationParams) (Conversation, []ConversationMember, error) {
	if err := s.validateContext(ctx, "create conversation"); err != nil {
		return Conversation{}, nil, err
	}
	params.OwnerAccountID = strings.TrimSpace(params.OwnerAccountID)
	params.Title = strings.TrimSpace(params.Title)
	params.Description = strings.TrimSpace(params.Description)
	params.AvatarMediaID = strings.TrimSpace(params.AvatarMediaID)
	params.MemberAccountIDs = uniqueIDs(params.MemberAccountIDs)
	params.Settings = normalizeConversationSettings(params.Settings)
	if params.OwnerAccountID == "" {
		return Conversation{}, nil, ErrInvalidInput
	}
	if params.Kind == ConversationKindUnspecified {
		return Conversation{}, nil, ErrInvalidInput
	}

	memberIDs, err := ensureMemberIDs(params.OwnerAccountID, params.MemberAccountIDs)
	if err != nil {
		return Conversation{}, nil, err
	}
	switch params.Kind {
	case ConversationKindSavedMessages:
		if len(memberIDs) != 0 {
			return Conversation{}, nil, ErrConflict
		}
	case ConversationKindDirect:
		if len(memberIDs) != 1 {
			return Conversation{}, nil, ErrConflict
		}
	}

	createdAt := params.CreatedAt
	if createdAt.IsZero() {
		createdAt = s.currentTime()
	}

	conversationID, err := newID("conv")
	if err != nil {
		return Conversation{}, nil, fmt.Errorf("generate conversation id: %w", err)
	}

	var (
		savedConversation Conversation
		savedMembers      []ConversationMember
	)

	err = s.runTx(ctx, func(tx Store) error {
		conversation := Conversation{
			ID:             conversationID,
			Kind:           params.Kind,
			Title:          params.Title,
			Description:    params.Description,
			AvatarMediaID:  params.AvatarMediaID,
			OwnerAccountID: params.OwnerAccountID,
			Settings:       params.Settings,
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt,
		}

		var saveErr error
		savedConversation, saveErr = tx.SaveConversation(ctx, conversation)
		if saveErr != nil {
			return fmt.Errorf("save conversation: %w", saveErr)
		}

		owner := ConversationMember{
			ConversationID: savedConversation.ID,
			AccountID:      params.OwnerAccountID,
			Role:           MemberRoleOwner,
			JoinedAt:       createdAt,
		}
		owner, saveErr = tx.SaveConversationMember(ctx, owner)
		if saveErr != nil {
			return fmt.Errorf("save owner membership: %w", saveErr)
		}
		savedMembers = append(savedMembers, owner)

		generalTopic := ConversationTopic{
			ConversationID:     savedConversation.ID,
			ID:                 "",
			Title:              generalTopicTitle,
			CreatedByAccountID: params.OwnerAccountID,
			IsGeneral:          true,
			CreatedAt:          createdAt,
			UpdatedAt:          createdAt,
		}
		if _, saveErr = tx.SaveTopic(ctx, generalTopic); saveErr != nil {
			return fmt.Errorf("save general topic: %w", saveErr)
		}

		for _, memberID := range memberIDs {
			member := ConversationMember{
				ConversationID: savedConversation.ID,
				AccountID:      memberID,
				Role:           MemberRoleMember,
				JoinedAt:       createdAt,
			}
			member, saveErr = tx.SaveConversationMember(ctx, member)
			if saveErr != nil {
				return fmt.Errorf("save member %s: %w", memberID, saveErr)
			}
			savedMembers = append(savedMembers, member)
		}

		event := EventEnvelope{
			EventID:        eventID(),
			EventType:      EventTypeConversationCreated,
			ConversationID: savedConversation.ID,
			ActorAccountID: params.OwnerAccountID,
			CreatedAt:      createdAt,
			Metadata: map[string]string{
				"kind":  string(params.Kind),
				"title": params.Title,
			},
		}
		event, saveErr = tx.SaveEvent(ctx, event)
		if saveErr != nil {
			return fmt.Errorf("save conversation event: %w", saveErr)
		}

		savedConversation.LastSequence = event.Sequence
		savedConversation.LastMessageAt = createdAt
		savedConversation.UpdatedAt = createdAt

		savedConversation, saveErr = tx.SaveConversation(ctx, savedConversation)
		if saveErr != nil {
			return fmt.Errorf("update conversation sequence: %w", saveErr)
		}

		return nil
	})
	if err != nil {
		return Conversation{}, nil, err
	}

	s.indexConversation(ctx, savedConversation, savedMembers)

	return savedConversation, savedMembers, nil
}

// GetConversation loads a conversation together with its members.
func (s *Service) GetConversation(ctx context.Context, params GetConversationParams) (Conversation, []ConversationMember, error) {
	if err := s.validateContext(ctx, "get conversation"); err != nil {
		return Conversation{}, nil, err
	}
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.ConversationID == "" {
		return Conversation{}, nil, ErrInvalidInput
	}

	conversation, err := s.store.ConversationByID(ctx, params.ConversationID)
	if err != nil {
		return Conversation{}, nil, fmt.Errorf("load conversation %s: %w", params.ConversationID, err)
	}
	members, err := s.store.ConversationMembersByConversationID(ctx, conversation.ID)
	if err != nil {
		return Conversation{}, nil, fmt.Errorf("load members for conversation %s: %w", conversation.ID, err)
	}
	if params.AccountID != "" {
		member, err := s.store.ConversationMemberByConversationAndAccount(ctx, conversation.ID, params.AccountID)
		if err != nil {
			return Conversation{}, nil, fmt.Errorf("authorize conversation %s for account %s: %w", conversation.ID, params.AccountID, err)
		}
		if !isActiveMember(member) {
			return Conversation{}, nil, ErrForbidden
		}
	}

	decorated, err := s.decorateConversationCounters(ctx, params.AccountID, []Conversation{conversation})
	if err != nil {
		return Conversation{}, nil, err
	}
	conversation = decorated[0]

	return conversation, members, nil
}

// ListConversations returns the conversations available to a member account.
func (s *Service) ListConversations(ctx context.Context, params ListConversationsParams) ([]Conversation, error) {
	if err := s.validateContext(ctx, "list conversations"); err != nil {
		return nil, err
	}
	params.AccountID = strings.TrimSpace(params.AccountID)
	if params.AccountID == "" {
		return nil, ErrInvalidInput
	}

	conversations, err := s.store.ConversationsByAccountID(ctx, params.AccountID)
	if err != nil {
		return nil, fmt.Errorf("load conversations for account %s: %w", params.AccountID, err)
	}

	filtered := make([]Conversation, 0, len(conversations))
	for _, conversation := range conversations {
		if !params.IncludeArchived && conversation.Archived {
			continue
		}
		if !params.IncludeMuted && conversation.Muted {
			continue
		}
		if !params.IncludeHidden && conversation.Hidden {
			continue
		}
		filtered = append(filtered, conversation)
	}

	slices.SortFunc(filtered, func(a, b Conversation) int {
		switch {
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

	filtered, err = s.decorateConversationCounters(ctx, params.AccountID, filtered)
	if err != nil {
		return nil, err
	}

	return filtered, nil
}
