package bot

import (
	"context"
	"strconv"
	"strings"
)

// PublicIDKind identifies one bot-facing public ID namespace.
type PublicIDKind string

// Supported public ID namespaces.
const (
	PublicIDKindAccount PublicIDKind = "account"
	PublicIDKindChat    PublicIDKind = "chat"
	PublicIDKindMessage PublicIDKind = "message"
	PublicIDKindTopic   PublicIDKind = "topic"
)

// PublicAccountID returns one stable bot-facing public account ID.
func (s *Service) PublicAccountID(ctx context.Context, accountID string) (int64, error) {
	return s.ensurePublicID(ctx, PublicIDKindAccount, accountID)
}

// PublicChatID returns one stable bot-facing public chat ID.
func (s *Service) PublicChatID(ctx context.Context, conversationID string) (int64, error) {
	return s.ensurePublicID(ctx, PublicIDKindChat, conversationID)
}

// PublicMessageID returns one stable bot-facing public message ID.
func (s *Service) PublicMessageID(ctx context.Context, messageID string) (int64, error) {
	return s.ensurePublicID(ctx, PublicIDKindMessage, messageID)
}

// PublicTopicID returns one stable bot-facing public topic ID.
func (s *Service) PublicTopicID(ctx context.Context, topicID string) (int64, error) {
	return s.ensurePublicID(ctx, PublicIDKindTopic, topicID)
}

// ResolveAccountID resolves a bot-facing public account ID back to the internal ID.
func (s *Service) ResolveAccountID(ctx context.Context, value string) (string, error) {
	return s.resolvePublicID(ctx, PublicIDKindAccount, value)
}

// ResolveChatID resolves a bot-facing public chat ID back to the internal ID.
func (s *Service) ResolveChatID(ctx context.Context, value string) (string, error) {
	return s.resolvePublicID(ctx, PublicIDKindChat, value)
}

// ResolveMessageID resolves a bot-facing public message ID back to the internal ID.
func (s *Service) ResolveMessageID(ctx context.Context, value string) (string, error) {
	return s.resolvePublicID(ctx, PublicIDKindMessage, value)
}

// ResolveTopicID resolves a bot-facing public topic ID back to the internal ID.
func (s *Service) ResolveTopicID(ctx context.Context, value string) (string, error) {
	return s.resolvePublicID(ctx, PublicIDKindTopic, value)
}

func (s *Service) ensurePublicID(ctx context.Context, kind PublicIDKind, internalID string) (int64, error) {
	if err := s.validateContext(ctx, "ensure public id"); err != nil {
		return 0, err
	}

	internalID = strings.TrimSpace(internalID)
	if kind == "" || internalID == "" {
		return 0, ErrInvalidInput
	}

	value, err := s.store.EnsurePublicID(ctx, kind, internalID)
	if err != nil {
		return 0, err
	}

	return value, nil
}

func (s *Service) resolvePublicID(ctx context.Context, kind PublicIDKind, value string) (string, error) {
	if err := s.validateContext(ctx, "resolve public id"); err != nil {
		return "", err
	}

	value = strings.TrimSpace(value)
	if kind == "" {
		return "", ErrInvalidInput
	}
	if value == "" {
		return "", nil
	}

	publicID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return value, nil
	}

	internalID, err := s.store.InternalIDByPublic(ctx, kind, publicID)
	if err != nil {
		return "", err
	}

	return internalID, nil
}
