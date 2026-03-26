package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
)

const (
	metadataCaptionKey = "bot.caption"
	metadataMediaIDKey = "bot.media_id"
)

type sendMediaParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	MediaID             string
	Caption             string
	ReplyToMessageID    string
	DisableNotification bool
	Method              conversation.MessageKind
}

// SendPhoto sends one image message as the authenticated bot.
func (s *Service) SendPhoto(ctx context.Context, params SendPhotoParams) (Message, error) {
	return s.sendMedia(ctx, sendMediaParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		MediaID:             params.MediaID,
		Caption:             params.Caption,
		ReplyToMessageID:    params.ReplyToMessageID,
		DisableNotification: params.DisableNotification,
		Method:              conversation.MessageKindImage,
	})
}

// SendDocument sends one document message as the authenticated bot.
func (s *Service) SendDocument(ctx context.Context, params SendDocumentParams) (Message, error) {
	return s.sendMedia(ctx, sendMediaParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		MediaID:             params.MediaID,
		Caption:             params.Caption,
		ReplyToMessageID:    params.ReplyToMessageID,
		DisableNotification: params.DisableNotification,
		Method:              conversation.MessageKindDocument,
	})
}

// SendVideo sends one video message as the authenticated bot.
func (s *Service) SendVideo(ctx context.Context, params SendVideoParams) (Message, error) {
	return s.sendMedia(ctx, sendMediaParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		MediaID:             params.MediaID,
		Caption:             params.Caption,
		ReplyToMessageID:    params.ReplyToMessageID,
		DisableNotification: params.DisableNotification,
		Method:              conversation.MessageKindVideo,
	})
}

// SendVoice sends one voice message as the authenticated bot.
func (s *Service) SendVoice(ctx context.Context, params SendVoiceParams) (Message, error) {
	return s.sendMedia(ctx, sendMediaParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		MediaID:             params.MediaID,
		Caption:             params.Caption,
		ReplyToMessageID:    params.ReplyToMessageID,
		DisableNotification: params.DisableNotification,
		Method:              conversation.MessageKindVoice,
	})
}

// SendSticker sends one sticker message as the authenticated bot.
func (s *Service) SendSticker(ctx context.Context, params SendStickerParams) (Message, error) {
	return s.sendMedia(ctx, sendMediaParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		MediaID:             params.MediaID,
		ReplyToMessageID:    params.ReplyToMessageID,
		DisableNotification: params.DisableNotification,
		Method:              conversation.MessageKindSticker,
	})
}

func (s *Service) sendMedia(ctx context.Context, params sendMediaParams) (Message, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return Message{}, err
	}

	params.ChatID = strings.TrimSpace(params.ChatID)
	params.MessageThreadID = strings.TrimSpace(params.MessageThreadID)
	params.MediaID = strings.TrimSpace(params.MediaID)
	params.Caption = strings.TrimSpace(params.Caption)
	params.ReplyToMessageID = strings.TrimSpace(params.ReplyToMessageID)
	if params.ChatID == "" || params.MediaID == "" || params.Method == conversation.MessageKindUnspecified {
		return Message{}, ErrInvalidInput
	}

	asset, err := s.media.MediaAssetByID(ctx, params.MediaID)
	if err != nil {
		return Message{}, fmt.Errorf("load bot media %s: %w", params.MediaID, mapMediaError(err))
	}
	if asset.OwnerAccountID != account.ID {
		return Message{}, ErrForbidden
	}
	if asset.Status != domainmedia.MediaStatusReady {
		return Message{}, ErrConflict
	}
	if !mediaMatchesMethod(asset.Kind, params.Method) {
		return Message{}, ErrInvalidInput
	}

	draft := conversation.MessageDraft{
		Kind:     params.Method,
		ThreadID: params.MessageThreadID,
		Silent:   params.DisableNotification,
		Payload:  mediaPayload(params.Method, params.Caption, params.MediaID),
		Metadata: mediaMetadata(params.Caption, params.MediaID),
		Attachments: []conversation.AttachmentRef{{
			MediaID:   asset.ID,
			Kind:      attachmentKindFromMedia(asset.Kind),
			FileName:  asset.FileName,
			MimeType:  asset.ContentType,
			SizeBytes: asset.SizeBytes,
			SHA256Hex: asset.SHA256Hex,
			Width:     asset.Width,
			Height:    asset.Height,
			Duration:  asset.Duration,
		}},
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
		return Message{}, fmt.Errorf("send bot media: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    params.ChatID,
		MessageID: message.ID,
	})
}

func mediaMatchesMethod(kind domainmedia.MediaKind, messageKind conversation.MessageKind) bool {
	switch messageKind {
	case conversation.MessageKindImage:
		return kind == domainmedia.MediaKindImage || kind == domainmedia.MediaKindAvatar
	case conversation.MessageKindDocument:
		return kind == domainmedia.MediaKindDocument || kind == domainmedia.MediaKindFile
	case conversation.MessageKindVideo:
		return kind == domainmedia.MediaKindVideo
	case conversation.MessageKindVoice:
		return kind == domainmedia.MediaKindVoice
	case conversation.MessageKindSticker:
		return kind == domainmedia.MediaKindSticker
	default:
		return false
	}
}

func attachmentKindFromMedia(kind domainmedia.MediaKind) conversation.AttachmentKind {
	switch kind {
	case domainmedia.MediaKindImage:
		return conversation.AttachmentKindImage
	case domainmedia.MediaKindVideo:
		return conversation.AttachmentKindVideo
	case domainmedia.MediaKindDocument:
		return conversation.AttachmentKindDocument
	case domainmedia.MediaKindVoice:
		return conversation.AttachmentKindVoice
	case domainmedia.MediaKindSticker:
		return conversation.AttachmentKindSticker
	case domainmedia.MediaKindGIF:
		return conversation.AttachmentKindGIF
	case domainmedia.MediaKindAvatar:
		return conversation.AttachmentKindAvatar
	case domainmedia.MediaKindFile:
		return conversation.AttachmentKindFile
	default:
		return conversation.AttachmentKindUnspecified
	}
}

func mediaPayload(kind conversation.MessageKind, caption string, mediaID string) conversation.EncryptedPayload {
	body := caption
	if body == "" {
		body = string(kind) + ":" + mediaID
	}

	return conversation.EncryptedPayload{
		Ciphertext: []byte(body),
	}
}

func mediaMetadata(caption string, mediaID string) map[string]string {
	metadata := map[string]string{
		metadataMediaIDKey: strings.TrimSpace(mediaID),
	}
	if strings.TrimSpace(caption) != "" {
		metadata[metadataCaptionKey] = strings.TrimSpace(caption)
	}

	return metadata
}

func mapMediaError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domainmedia.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, domainmedia.ErrForbidden):
		return ErrForbidden
	case errors.Is(err, domainmedia.ErrConflict):
		return ErrConflict
	case errors.Is(err, domainmedia.ErrInvalidInput):
		return ErrInvalidInput
	default:
		return err
	}
}
