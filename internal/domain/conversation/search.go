package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
)

func (s *Service) indexConversation(ctx context.Context, conversation Conversation, members []ConversationMember) {
	if s == nil || s.indexer == nil || conversation.ID == "" {
		return
	}

	document := domainsearch.Document{
		Scope:          domainsearch.SearchScopeConversations,
		EntityType:     "conversation",
		TargetID:       conversation.ID,
		Title:          conversation.Title,
		Subtitle:       conversation.Description,
		Snippet:        conversationSearchSnippet(conversation, members),
		ConversationID: conversation.ID,
		Metadata:       conversationSearchMetadata(conversation, members),
		UpdatedAt:      conversation.UpdatedAt,
		CreatedAt:      conversation.CreatedAt,
	}
	if document.Title == "" {
		document.Title = string(conversation.Kind)
	}

	_, _ = s.indexer.UpsertDocument(ctx, document)
}

func (s *Service) indexConversationByID(ctx context.Context, conversationID string) {
	if s == nil || s.indexer == nil || strings.TrimSpace(conversationID) == "" {
		return
	}

	conversation, err := s.store.ConversationByID(ctx, conversationID)
	if err != nil {
		return
	}
	members, err := s.store.ConversationMembersByConversationID(ctx, conversationID)
	if err != nil {
		return
	}

	s.indexConversation(ctx, conversation, members)
}

func (s *Service) indexTopic(ctx context.Context, topic ConversationTopic) {
	if s == nil || s.indexer == nil || topic.ConversationID == "" {
		return
	}

	targetID := topicIndexID(topic.ConversationID, topic.ID, topic.IsGeneral)
	document := domainsearch.Document{
		Scope:          domainsearch.SearchScopeConversations,
		EntityType:     "topic",
		TargetID:       targetID,
		Title:          topic.Title,
		Subtitle:       topicSubtitle(topic),
		Snippet:        topicSnippet(topic),
		ConversationID: topic.ConversationID,
		Metadata:       topicSearchMetadata(topic),
		UpdatedAt:      topic.UpdatedAt,
		CreatedAt:      topic.CreatedAt,
	}
	_, _ = s.indexer.UpsertDocument(ctx, document)
}

func (s *Service) deleteMessageIndex(ctx context.Context, message Message) {
	if s == nil || s.indexer == nil || message.ID == "" {
		return
	}

	_ = s.indexer.DeleteDocument(ctx, domainsearch.SearchScopeMessages, message.ID)
}

func (s *Service) indexMessage(ctx context.Context, message Message) {
	if s == nil || s.indexer == nil || message.ID == "" {
		return
	}

	document := domainsearch.Document{
		Scope:          domainsearch.SearchScopeMessages,
		EntityType:     "message",
		TargetID:       message.ID,
		Title:          strings.ToLower(string(message.Kind)),
		Subtitle:       message.ThreadID,
		Snippet:        messageSearchSnippet(message),
		ConversationID: message.ConversationID,
		MessageID:      message.ID,
		UserID:         message.SenderAccountID,
		Metadata:       messageSearchMetadata(message),
		UpdatedAt:      message.UpdatedAt,
		CreatedAt:      message.CreatedAt,
	}
	_, _ = s.indexer.UpsertDocument(ctx, document)
}

func conversationSearchSnippet(conversation Conversation, members []ConversationMember) string {
	parts := []string{
		string(conversation.Kind),
		conversation.OwnerAccountID,
	}
	if conversation.Description != "" {
		parts = append(parts, conversation.Description)
	}
	if len(members) > 0 {
		parts = append(parts, fmt.Sprintf("%d members", len(members)))
	}

	return strings.Join(parts, " ")
}

func conversationSearchMetadata(conversation Conversation, members []ConversationMember) map[string]string {
	metadata := map[string]string{
		"kind":                  strings.ToLower(string(conversation.Kind)),
		"owner_account_id":      conversation.OwnerAccountID,
		"member_count":          fmt.Sprintf("%d", len(members)),
		"allow_threads":         fmt.Sprintf("%t", conversation.Settings.AllowThreads),
		"allow_reactions":       fmt.Sprintf("%t", conversation.Settings.AllowReactions),
		"require_encrypted":     fmt.Sprintf("%t", conversation.Settings.RequireEncryptedMessages),
		"only_admins_can_write": fmt.Sprintf("%t", conversation.Settings.OnlyAdminsCanWrite),
		"pinned_only_admins":    fmt.Sprintf("%t", conversation.Settings.PinnedMessagesOnlyAdmins),
		"only_admins_can_add":   fmt.Sprintf("%t", conversation.Settings.OnlyAdminsCanAddMembers),
		"require_join_approval": fmt.Sprintf("%t", conversation.Settings.RequireJoinApproval),
		"allow_forwards":        fmt.Sprintf("%t", conversation.Settings.AllowForwards),
		"archived":              fmt.Sprintf("%t", conversation.Archived),
		"muted":                 fmt.Sprintf("%t", conversation.Muted),
		"pinned":                fmt.Sprintf("%t", conversation.Pinned),
		"hidden":                fmt.Sprintf("%t", conversation.Hidden),
	}
	if !conversation.LastMessageAt.IsZero() {
		metadata["last_message_at"] = conversation.LastMessageAt.UTC().Format(time.RFC3339Nano)
	}

	return metadata
}

func topicIndexID(conversationID string, topicID string, isGeneral bool) string {
	conversationID = strings.TrimSpace(conversationID)
	topicID = strings.TrimSpace(topicID)
	if isGeneral || topicID == "" {
		return conversationID + ":general"
	}

	return conversationID + ":" + topicID
}

func topicSubtitle(topic ConversationTopic) string {
	if topic.IsGeneral {
		return generalTopicTitle
	}

	return topic.CreatedByAccountID
}

func topicSnippet(topic ConversationTopic) string {
	parts := []string{topic.CreatedByAccountID}
	if topic.IsGeneral {
		parts = append(parts, "general")
	}
	if topic.Archived {
		parts = append(parts, "archived")
	}
	if topic.Closed {
		parts = append(parts, "closed")
	}
	if topic.Pinned {
		parts = append(parts, "pinned")
	}

	return strings.Join(parts, " ")
}

func topicSearchMetadata(topic ConversationTopic) map[string]string {
	metadata := map[string]string{
		"topic_id":      topic.ID,
		"is_general":    fmt.Sprintf("%t", topic.IsGeneral),
		"archived":      fmt.Sprintf("%t", topic.Archived),
		"pinned":        fmt.Sprintf("%t", topic.Pinned),
		"closed":        fmt.Sprintf("%t", topic.Closed),
		"message_count": fmt.Sprintf("%d", topic.MessageCount),
	}
	if !topic.LastMessageAt.IsZero() {
		metadata["last_message_at"] = topic.LastMessageAt.UTC().Format(time.RFC3339Nano)
	}

	return metadata
}

func messageSearchSnippet(message Message) string {
	parts := make([]string, 0, len(message.Attachments)+len(message.MentionAccountIDs)+4)
	parts = append(parts, message.SenderAccountID, message.ClientMessageID, message.ThreadID)
	for _, attachment := range message.Attachments {
		if attachment.FileName != "" {
			parts = append(parts, attachment.FileName)
		}
		if attachment.MimeType != "" {
			parts = append(parts, attachment.MimeType)
		}
	}
	parts = append(parts, message.MentionAccountIDs...)
	if message.ReplyTo.MessageID != "" {
		parts = append(parts, message.ReplyTo.MessageID)
	}

	return strings.Join(parts, " ")
}

func messageSearchMetadata(message Message) map[string]string {
	metadata := map[string]string{
		"conversation_id":       message.ConversationID,
		"sender_account_id":     message.SenderAccountID,
		"sender_device_id":      message.SenderDeviceID,
		"client_message_id":     message.ClientMessageID,
		"thread_id":             message.ThreadID,
		"kind":                  strings.ToLower(string(message.Kind)),
		"status":                strings.ToLower(string(message.Status)),
		"silent":                fmt.Sprintf("%t", message.Silent),
		"pinned":                fmt.Sprintf("%t", message.Pinned),
		"disable_link_previews": fmt.Sprintf("%t", message.DisableLinkPreviews),
		"attachment_count":      fmt.Sprintf("%d", len(message.Attachments)),
		"mention_count":         fmt.Sprintf("%d", len(message.MentionAccountIDs)),
		"reaction_count":        fmt.Sprintf("%d", len(message.Reactions)),
	}
	if message.ReplyTo.MessageID != "" {
		metadata["reply_message_id"] = message.ReplyTo.MessageID
		metadata["reply_sender_account_id"] = message.ReplyTo.SenderAccountID
		metadata["reply_kind"] = strings.ToLower(string(message.ReplyTo.MessageKind))
	}

	return metadata
}
