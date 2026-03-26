package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// ForwardMessageParams describes one forwardMessage request.
type ForwardMessageParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	FromChatID          string
	MessageID           string
	DisableNotification bool
}

// CopyMessageParams describes one copyMessage request.
type CopyMessageParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	FromChatID          string
	MessageID           string
	Caption             string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// ForwardMessage re-sends one visible message into the destination chat.
func (s *Service) ForwardMessage(ctx context.Context, params ForwardMessageParams) (Message, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return Message{}, err
	}

	targetChatID := strings.TrimSpace(params.ChatID)
	targetThreadID := strings.TrimSpace(params.MessageThreadID)
	sourceChatID := strings.TrimSpace(params.FromChatID)
	sourceMessageID := strings.TrimSpace(params.MessageID)
	if targetChatID == "" || sourceChatID == "" || sourceMessageID == "" {
		return Message{}, ErrInvalidInput
	}

	source, err := s.conversations.GetMessage(ctx, conversation.GetMessageParams{
		ConversationID: sourceChatID,
		MessageID:      sourceMessageID,
		AccountID:      account.ID,
	})
	if err != nil {
		return Message{}, fmt.Errorf("load forwarded bot message: %w", mapConversationError(err))
	}

	message, _, err := s.conversations.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  targetChatID,
		SenderAccountID: account.ID,
		SenderDeviceID:  botDeviceID,
		Draft:           copyDraft(source, targetThreadID, params.DisableNotification, source.Metadata),
	})
	if err != nil {
		return Message{}, fmt.Errorf("forward bot message: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    targetChatID,
		MessageID: message.ID,
	})
}

// CopyMessage copies one visible message into the destination chat.
func (s *Service) CopyMessage(ctx context.Context, params CopyMessageParams) (string, error) {
	return s.copyMessage(ctx, params, false)
}

func (s *Service) copyMessage(ctx context.Context, params CopyMessageParams, removeCaption bool) (string, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return "", err
	}

	targetChatID := strings.TrimSpace(params.ChatID)
	targetThreadID := strings.TrimSpace(params.MessageThreadID)
	sourceChatID := strings.TrimSpace(params.FromChatID)
	sourceMessageID := strings.TrimSpace(params.MessageID)
	replyToMessageID := strings.TrimSpace(params.ReplyToMessageID)
	if targetChatID == "" || sourceChatID == "" || sourceMessageID == "" {
		return "", ErrInvalidInput
	}

	source, err := s.conversations.GetMessage(ctx, conversation.GetMessageParams{
		ConversationID: sourceChatID,
		MessageID:      sourceMessageID,
		AccountID:      account.ID,
	})
	if err != nil {
		return "", fmt.Errorf("load copied bot message: %w", mapConversationError(err))
	}

	replyMarkup := params.ReplyMarkup
	if replyMarkup == nil {
		replyMarkup = messageReplyMarkup(source.Metadata)
	}
	caption := strings.TrimSpace(params.Caption)
	if caption == "" && !removeCaption {
		caption = strings.TrimSpace(source.Metadata[metadataCaptionKey])
	}
	metadata, err := markupMetadata(withCaption(withoutMarkup(source.Metadata), params.Caption), replyMarkup)
	if err != nil {
		return "", err
	}
	metadata = withCaption(metadata, caption)

	draft := copyDraft(source, targetThreadID, params.DisableNotification, metadata)

	var message conversation.Message
	if replyToMessageID != "" {
		message, _, err = s.conversations.ReplyMessage(ctx, conversation.ReplyMessageParams{
			ConversationID:   targetChatID,
			SenderAccountID:  account.ID,
			SenderDeviceID:   botDeviceID,
			ReplyToMessageID: replyToMessageID,
			Draft:            draft,
		})
	} else {
		message, _, err = s.conversations.SendMessage(ctx, conversation.SendMessageParams{
			ConversationID:  targetChatID,
			SenderAccountID: account.ID,
			SenderDeviceID:  botDeviceID,
			Draft:           draft,
		})
	}
	if err != nil {
		return "", fmt.Errorf("copy bot message: %w", mapConversationError(err))
	}

	return message.ID, nil
}

// ForwardMessages re-sends multiple visible messages into the destination chat.
func (s *Service) ForwardMessages(ctx context.Context, params ForwardMessagesParams) ([]string, error) {
	params.ChatID = strings.TrimSpace(params.ChatID)
	params.MessageThreadID = strings.TrimSpace(params.MessageThreadID)
	params.FromChatID = strings.TrimSpace(params.FromChatID)
	if params.ChatID == "" || params.FromChatID == "" || len(params.MessageIDs) == 0 {
		return nil, ErrInvalidInput
	}

	result := make([]string, 0, len(params.MessageIDs))
	for _, messageID := range params.MessageIDs {
		messageID = strings.TrimSpace(messageID)
		if messageID == "" {
			return nil, ErrInvalidInput
		}

		message, err := s.ForwardMessage(ctx, ForwardMessageParams{
			BotToken:            params.BotToken,
			ChatID:              params.ChatID,
			MessageThreadID:     params.MessageThreadID,
			FromChatID:          params.FromChatID,
			MessageID:           messageID,
			DisableNotification: params.DisableNotification,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, message.MessageID)
	}

	return result, nil
}

// CopyMessages copies multiple visible messages into the destination chat.
func (s *Service) CopyMessages(ctx context.Context, params CopyMessagesParams) ([]string, error) {
	params.ChatID = strings.TrimSpace(params.ChatID)
	params.MessageThreadID = strings.TrimSpace(params.MessageThreadID)
	params.FromChatID = strings.TrimSpace(params.FromChatID)
	if params.ChatID == "" || params.FromChatID == "" || len(params.MessageIDs) == 0 {
		return nil, ErrInvalidInput
	}

	result := make([]string, 0, len(params.MessageIDs))
	for _, messageID := range params.MessageIDs {
		messageID = strings.TrimSpace(messageID)
		if messageID == "" {
			return nil, ErrInvalidInput
		}

		copiedID, err := s.copyMessage(ctx, CopyMessageParams{
			BotToken:            params.BotToken,
			ChatID:              params.ChatID,
			MessageThreadID:     params.MessageThreadID,
			FromChatID:          params.FromChatID,
			MessageID:           messageID,
			DisableNotification: params.DisableNotification,
		}, params.RemoveCaption)
		if err != nil {
			return nil, err
		}
		result = append(result, copiedID)
	}

	return result, nil
}
