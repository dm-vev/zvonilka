package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SendMessage sends one text message as the authenticated bot.
func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) (Message, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return Message{}, err
	}

	params.ChatID = strings.TrimSpace(params.ChatID)
	params.MessageThreadID = strings.TrimSpace(params.MessageThreadID)
	params.ReplyToMessageID = strings.TrimSpace(params.ReplyToMessageID)
	if params.ChatID == "" || params.Text == "" {
		return Message{}, ErrInvalidInput
	}

	draft := conversation.MessageDraft{
		Kind: conversation.MessageKindText,
		Payload: conversation.EncryptedPayload{
			Ciphertext: []byte(params.Text),
		},
		ThreadID:            params.MessageThreadID,
		Silent:              params.DisableNotification,
		DisableLinkPreviews: params.DisableWebPagePreview,
	}

	var message conversation.Message
	if params.ReplyToMessageID != "" {
		message, _, err = s.conversations.ReplyMessage(ctx, conversation.ReplyMessageParams{
			ConversationID:   params.ChatID,
			SenderAccountID:  account.ID,
			SenderDeviceID:   botDeviceID,
			ReplyToMessageID: params.ReplyToMessageID,
			Draft:            draft,
		})
	} else {
		message, _, err = s.conversations.SendMessage(ctx, conversation.SendMessageParams{
			ConversationID:  params.ChatID,
			SenderAccountID: account.ID,
			SenderDeviceID:  botDeviceID,
			Draft:           draft,
		})
	}
	if err != nil {
		return Message{}, fmt.Errorf("send bot message: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    params.ChatID,
		MessageID: message.ID,
	})
}

// EditMessageText edits one previously sent text message.
func (s *Service) EditMessageText(ctx context.Context, params EditMessageTextParams) (Message, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return Message{}, err
	}

	params.ChatID = strings.TrimSpace(params.ChatID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	if params.ChatID == "" || params.MessageID == "" || params.Text == "" {
		return Message{}, ErrInvalidInput
	}

	message, _, err := s.conversations.EditMessage(ctx, conversation.EditMessageParams{
		ConversationID: params.ChatID,
		MessageID:      params.MessageID,
		ActorAccountID: account.ID,
		ActorDeviceID:  botDeviceID,
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte(params.Text),
			},
			DisableLinkPreviews: params.DisableWebPagePreview,
		},
	})
	if err != nil {
		return Message{}, fmt.Errorf("edit bot message: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    params.ChatID,
		MessageID: message.ID,
	})
}

// DeleteMessage deletes one bot-visible conversation message.
func (s *Service) DeleteMessage(ctx context.Context, params DeleteMessageParams) error {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return err
	}

	params.ChatID = strings.TrimSpace(params.ChatID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	if params.ChatID == "" || params.MessageID == "" {
		return ErrInvalidInput
	}

	_, _, err = s.conversations.DeleteMessage(ctx, conversation.DeleteMessageParams{
		ConversationID: params.ChatID,
		MessageID:      params.MessageID,
		ActorAccountID: account.ID,
		ActorDeviceID:  botDeviceID,
	})
	if err != nil {
		return fmt.Errorf("delete bot message: %w", mapConversationError(err))
	}

	return nil
}

// GetChat resolves one bot-visible chat projection.
func (s *Service) GetChat(ctx context.Context, params GetChatParams) (Chat, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return Chat{}, err
	}

	params.ChatID = strings.TrimSpace(params.ChatID)
	if params.ChatID == "" {
		return Chat{}, ErrInvalidInput
	}

	conv, members, err := s.conversations.GetConversation(ctx, conversation.GetConversationParams{
		ConversationID: params.ChatID,
		AccountID:      account.ID,
	})
	if err != nil {
		return Chat{}, fmt.Errorf("get bot chat: %w", mapConversationError(err))
	}

	return s.chatForConversation(ctx, account.ID, conv, members)
}

// GetChatMember resolves one chat-member projection visible to the bot.
func (s *Service) GetChatMember(ctx context.Context, params GetChatMemberParams) (ChatMember, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return ChatMember{}, err
	}

	params.ChatID = strings.TrimSpace(params.ChatID)
	params.UserID = strings.TrimSpace(params.UserID)
	if params.ChatID == "" || params.UserID == "" {
		return ChatMember{}, ErrInvalidInput
	}

	members, err := s.conversations.ListMembers(ctx, conversation.GetConversationParams{
		ConversationID: params.ChatID,
		AccountID:      account.ID,
	})
	if err != nil {
		return ChatMember{}, fmt.Errorf("list bot chat members: %w", mapConversationError(err))
	}

	var target conversation.ConversationMember
	found := false
	for _, member := range members {
		if member.AccountID != params.UserID {
			continue
		}
		target = member
		found = true
		break
	}
	if !found {
		return ChatMember{}, ErrNotFound
	}

	userAccount, err := s.identity.AccountByID(ctx, target.AccountID)
	if err != nil {
		return ChatMember{}, fmt.Errorf("load chat member account: %w", mapIdentityError(err))
	}

	restricted, err := s.isRestricted(ctx, params.ChatID, target.AccountID)
	if err != nil {
		return ChatMember{}, err
	}

	return ChatMember{
		User:   userFromAccount(userAccount),
		Status: memberStatus(target, restricted),
	}, nil
}

// GetMessage resolves one bot-visible message projection.
func (s *Service) GetMessage(ctx context.Context, params GetMessageParams) (Message, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return Message{}, err
	}

	params.ChatID = strings.TrimSpace(params.ChatID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	if params.ChatID == "" || params.MessageID == "" {
		return Message{}, ErrInvalidInput
	}

	conv, members, err := s.conversations.GetConversation(ctx, conversation.GetConversationParams{
		ConversationID: params.ChatID,
		AccountID:      account.ID,
	})
	if err != nil {
		return Message{}, fmt.Errorf("load message conversation: %w", mapConversationError(err))
	}
	message, err := s.conversations.GetMessage(ctx, conversation.GetMessageParams{
		ConversationID: params.ChatID,
		MessageID:      params.MessageID,
		AccountID:      account.ID,
	})
	if err != nil {
		return Message{}, fmt.Errorf("load bot message: %w", mapConversationError(err))
	}

	return s.messageForConversation(ctx, account.ID, conv, members, message, true)
}

func (s *Service) isRestricted(ctx context.Context, conversationID string, accountID string) (bool, error) {
	restriction, err := s.conversationDB.ModerationRestrictionByTargetAndAccount(
		ctx,
		conversation.ModerationTargetKindConversation,
		conversationID,
		accountID,
	)
	if err == nil {
		return restriction.State == conversation.ModerationRestrictionStateMuted, nil
	}
	if !errors.Is(err, conversation.ErrNotFound) {
		return false, fmt.Errorf("load moderation restriction for %s/%s: %w", conversationID, accountID, err)
	}

	return false, nil
}
