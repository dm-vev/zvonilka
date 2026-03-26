package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

// EditCaptionParams describes one editMessageCaption request.
type EditCaptionParams struct {
	BotToken    string
	ChatID      string
	MessageID   string
	Caption     string
	ReplyMarkup *InlineKeyboardMarkup
}

// EditMarkupParams describes one editMessageReplyMarkup request.
type EditMarkupParams struct {
	BotToken    string
	ChatID      string
	MessageID   string
	ReplyMarkup *InlineKeyboardMarkup
}

// EditMessageMedia edits media payload, caption, and optional markup for one bot message.
func (s *Service) EditMessageMedia(ctx context.Context, params EditMediaParams) (Message, error) {
	accountID, conv, _, raw, err := s.loadRawMessage(ctx, params.BotToken, params.ChatID, params.MessageID)
	if err != nil {
		return Message{}, err
	}
	if !editableMessage(raw) {
		return Message{}, ErrInvalidInput
	}

	params.MediaID = strings.TrimSpace(params.MediaID)
	params.Shape = strings.TrimSpace(params.Shape)
	if params.MediaID == "" || params.Shape == "" {
		return Message{}, ErrInvalidInput
	}

	method, ok := editableMediaMethod(params.Shape)
	if !ok {
		return Message{}, ErrInvalidInput
	}

	asset, err := s.media.MediaAssetByID(ctx, params.MediaID)
	if err != nil {
		return Message{}, fmt.Errorf("load bot media %s: %w", params.MediaID, mapMediaError(err))
	}
	if asset.OwnerAccountID != accountID {
		return Message{}, ErrForbidden
	}
	if asset.Status != domainmedia.MediaStatusReady {
		return Message{}, ErrConflict
	}
	if !mediaMatchesMethod(asset.Kind, method, params.Shape) {
		return Message{}, ErrInvalidInput
	}

	replyMarkup := params.ReplyMarkup
	if replyMarkup == nil {
		replyMarkup = messageReplyMarkup(raw.Metadata)
	}

	caption := messageCaption(raw)
	if params.Caption != nil {
		caption = strings.TrimSpace(*params.Caption)
	}

	metadata, err := markupMetadata(mediaMetadata(caption, params.MediaID, params.Shape), replyMarkup)
	if err != nil {
		return Message{}, err
	}

	draft := copyDraft(raw, raw.ThreadID, raw.Silent, metadata)
	draft.Kind = method
	draft.Payload = mediaPayload(method, caption, params.MediaID)
	draft.Attachments = []conversation.AttachmentRef{{
		MediaID:   asset.ID,
		Kind:      attachmentKindFromMedia(asset.Kind),
		FileName:  asset.FileName,
		MimeType:  asset.ContentType,
		SizeBytes: asset.SizeBytes,
		SHA256Hex: asset.SHA256Hex,
		Width:     asset.Width,
		Height:    asset.Height,
		Duration:  asset.Duration,
	}}

	message, _, err := s.conversations.EditMessage(ctx, conversation.EditMessageParams{
		ConversationID: conv.ID,
		MessageID:      raw.ID,
		ActorAccountID: accountID,
		ActorDeviceID:  botDeviceID,
		Draft:          draft,
	})
	if err != nil {
		return Message{}, fmt.Errorf("edit bot message media: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    conv.ID,
		MessageID: message.ID,
	})
}

// EditMessageCaption edits caption and optionally markup for one bot message.
func (s *Service) EditMessageCaption(ctx context.Context, params EditCaptionParams) (Message, error) {
	accountID, conv, _, raw, err := s.loadRawMessage(ctx, params.BotToken, params.ChatID, params.MessageID)
	if err != nil {
		return Message{}, err
	}
	if !supportsCaption(raw) {
		return Message{}, ErrInvalidInput
	}

	replyMarkup := params.ReplyMarkup
	if replyMarkup == nil {
		replyMarkup = messageReplyMarkup(raw.Metadata)
	}
	metadata, err := markupMetadata(withCaption(withoutMarkup(raw.Metadata), params.Caption), replyMarkup)
	if err != nil {
		return Message{}, err
	}

	message, _, err := s.conversations.EditMessage(ctx, conversation.EditMessageParams{
		ConversationID: conv.ID,
		MessageID:      raw.ID,
		ActorAccountID: accountID,
		ActorDeviceID:  botDeviceID,
		Draft:          copyDraft(raw, raw.ThreadID, raw.Silent, metadata),
	})
	if err != nil {
		return Message{}, fmt.Errorf("edit bot message caption: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    conv.ID,
		MessageID: message.ID,
	})
}

// EditMessageReplyMarkup edits inline keyboard markup for one bot message.
func (s *Service) EditMessageReplyMarkup(ctx context.Context, params EditMarkupParams) (Message, error) {
	accountID, conv, _, raw, err := s.loadRawMessage(ctx, params.BotToken, params.ChatID, params.MessageID)
	if err != nil {
		return Message{}, err
	}

	metadata, err := markupMetadata(withoutMarkup(raw.Metadata), params.ReplyMarkup)
	if err != nil {
		return Message{}, err
	}

	message, _, err := s.conversations.EditMessage(ctx, conversation.EditMessageParams{
		ConversationID: conv.ID,
		MessageID:      raw.ID,
		ActorAccountID: accountID,
		ActorDeviceID:  botDeviceID,
		Draft:          copyDraft(raw, raw.ThreadID, raw.Silent, metadata),
	})
	if err != nil {
		return Message{}, fmt.Errorf("edit bot message reply markup: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    conv.ID,
		MessageID: message.ID,
	})
}

func (s *Service) loadRawMessage(
	ctx context.Context,
	botToken string,
	chatID string,
	messageID string,
) (accountID string, conv conversation.Conversation, members []conversation.ConversationMember, message conversation.Message, err error) {
	account, err := s.botAccount(ctx, botToken)
	if err != nil {
		return "", conversation.Conversation{}, nil, conversation.Message{}, err
	}

	chatID = strings.TrimSpace(chatID)
	messageID = strings.TrimSpace(messageID)
	if chatID == "" || messageID == "" {
		return "", conversation.Conversation{}, nil, conversation.Message{}, ErrInvalidInput
	}

	conv, members, err = s.conversations.GetConversation(ctx, conversation.GetConversationParams{
		ConversationID: chatID,
		AccountID:      account.ID,
	})
	if err != nil {
		return "", conversation.Conversation{}, nil, conversation.Message{}, fmt.Errorf("load bot conversation: %w", mapConversationError(err))
	}
	message, err = s.conversations.GetMessage(ctx, conversation.GetMessageParams{
		ConversationID: chatID,
		MessageID:      messageID,
		AccountID:      account.ID,
	})
	if err != nil {
		return "", conversation.Conversation{}, nil, conversation.Message{}, fmt.Errorf("load bot message: %w", mapConversationError(err))
	}

	return account.ID, conv, members, message, nil
}
