package bot

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

const botDeviceID = "botapi"

func userFromAccount(account identity.Account) User {
	firstName := strings.TrimSpace(account.DisplayName)
	if firstName == "" {
		firstName = strings.TrimSpace(account.Username)
	}
	if firstName == "" {
		firstName = account.ID
	}

	return User{
		ID:                      account.ID,
		IsBot:                   account.Kind == identity.AccountKindBot,
		FirstName:               firstName,
		Username:                account.Username,
		CanJoinGroups:           true,
		CanReadAllGroupMessages: false,
		SupportsInlineQueries:   false,
	}
}

func chatTypeFromConversation(conv conversation.Conversation) ChatType {
	switch conv.Kind {
	case conversation.ConversationKindDirect:
		return ChatTypePrivate
	case conversation.ConversationKindGroup:
		if conv.Settings.AllowThreads {
			return ChatTypeSupergroup
		}
		return ChatTypeGroup
	case conversation.ConversationKindChannel:
		return ChatTypeChannel
	default:
		return ChatTypeUnspecified
	}
}

func plainText(message conversation.Message) string {
	if message.Kind != conversation.MessageKindText {
		return ""
	}
	if strings.TrimSpace(message.Payload.KeyID) != "" || strings.TrimSpace(message.Payload.Algorithm) != "" {
		return ""
	}
	if len(message.Payload.Nonce) > 0 || len(message.Payload.AAD) > 0 {
		return ""
	}
	if !utf8.Valid(message.Payload.Ciphertext) {
		return ""
	}

	return string(message.Payload.Ciphertext)
}

func messageCaption(message conversation.Message) string {
	return strings.TrimSpace(message.Metadata[metadataCaptionKey])
}

func messageMediaID(message conversation.Message) string {
	if mediaID := strings.TrimSpace(message.Metadata[metadataMediaIDKey]); mediaID != "" {
		return mediaID
	}
	if len(message.Attachments) == 0 {
		return ""
	}

	return strings.TrimSpace(message.Attachments[0].MediaID)
}

func messageShape(message conversation.Message) string {
	shape := strings.TrimSpace(message.Metadata[metadataShapeKey])
	if shape != "" {
		return shape
	}

	switch message.Kind {
	case conversation.MessageKindImage:
		return "photo"
	case conversation.MessageKindDocument:
		return "document"
	case conversation.MessageKindVideo:
		return "video"
	case conversation.MessageKindVoice:
		return "voice"
	case conversation.MessageKindSticker:
		return "sticker"
	case conversation.MessageKindGIF:
		return "animation"
	default:
		return ""
	}
}

func messageMedia(message conversation.Message) ([]PhotoSize, *Document, *Video, *Animation, *Audio, *VideoNote, *Voice, *Sticker) {
	if len(message.Attachments) == 0 {
		return nil, nil, nil, nil, nil, nil, nil, nil
	}

	attachment := message.Attachments[0]
	file := File{
		FileID:       messageMediaID(message),
		FileUniqueID: messageMediaID(message),
		FileSize:     attachment.SizeBytes,
	}

	switch messageShape(message) {
	case "photo":
		return []PhotoSize{{
			File:   file,
			Width:  attachment.Width,
			Height: attachment.Height,
		}}, nil, nil, nil, nil, nil, nil, nil
	case "document":
		return nil, &Document{
			File:     file,
			FileName: attachment.FileName,
			MimeType: attachment.MimeType,
		}, nil, nil, nil, nil, nil, nil
	case "video":
		return nil, nil, &Video{
			File:     file,
			Width:    attachment.Width,
			Height:   attachment.Height,
			Duration: int(attachment.Duration.Seconds()),
			MimeType: attachment.MimeType,
		}, nil, nil, nil, nil, nil
	case "animation":
		return nil, nil, nil, &Animation{
			File:     file,
			Width:    attachment.Width,
			Height:   attachment.Height,
			Duration: int(attachment.Duration.Seconds()),
			FileName: attachment.FileName,
			MimeType: attachment.MimeType,
		}, nil, nil, nil, nil
	case "audio":
		return nil, nil, nil, nil, &Audio{
			File:     file,
			Duration: int(attachment.Duration.Seconds()),
			FileName: attachment.FileName,
			MimeType: attachment.MimeType,
		}, nil, nil, nil
	case "video_note":
		length := attachment.Width
		if attachment.Height > length {
			length = attachment.Height
		}
		return nil, nil, nil, nil, nil, &VideoNote{
			File:     file,
			Length:   length,
			Duration: int(attachment.Duration.Seconds()),
		}, nil, nil
	case "voice":
		return nil, nil, nil, nil, nil, nil, &Voice{
			File:     file,
			Duration: int(attachment.Duration.Seconds()),
			MimeType: attachment.MimeType,
		}, nil
	case "sticker":
		return nil, nil, nil, nil, nil, nil, nil, &Sticker{
			File:     file,
			Width:    attachment.Width,
			Height:   attachment.Height,
			MimeType: attachment.MimeType,
		}
	default:
		return nil, nil, nil, nil, nil, nil, nil, nil
	}
}

func (s *Service) chatForConversation(
	ctx context.Context,
	botAccountID string,
	conv conversation.Conversation,
	members []conversation.ConversationMember,
) (Chat, error) {
	chat := Chat{
		ID:      conv.ID,
		Type:    chatTypeFromConversation(conv),
		Title:   conv.Title,
		IsForum: conv.Kind == conversation.ConversationKindGroup && conv.Settings.AllowThreads,
	}
	if conv.Kind != conversation.ConversationKindDirect {
		return chat, nil
	}

	for _, member := range members {
		if member.AccountID == botAccountID {
			continue
		}
		account, err := s.identity.AccountByID(ctx, member.AccountID)
		if err != nil {
			return Chat{}, mapIdentityError(err)
		}
		chat.Title = strings.TrimSpace(account.DisplayName)
		if chat.Title == "" {
			chat.Title = account.Username
		}
		chat.Username = account.Username
		return chat, nil
	}

	return chat, nil
}

func memberStatus(member conversation.ConversationMember, restricted bool) MemberStatus {
	if member.Banned {
		return MemberStatusKicked
	}
	if !member.LeftAt.IsZero() {
		return MemberStatusLeft
	}
	if restricted {
		return MemberStatusRestricted
	}
	switch member.Role {
	case conversation.MemberRoleOwner:
		return MemberStatusCreator
	case conversation.MemberRoleAdmin:
		return MemberStatusAdministrator
	default:
		return MemberStatusMember
	}
}

func (s *Service) messageForConversation(
	ctx context.Context,
	botAccountID string,
	conv conversation.Conversation,
	members []conversation.ConversationMember,
	msg conversation.Message,
	includeReply bool,
) (Message, error) {
	chat, err := s.chatForConversation(ctx, botAccountID, conv, members)
	if err != nil {
		return Message{}, err
	}

	sender, err := s.identity.AccountByID(ctx, msg.SenderAccountID)
	if err != nil {
		return Message{}, mapIdentityError(err)
	}

	result := Message{
		MessageID:   msg.ID,
		Date:        msg.CreatedAt.UTC().Unix(),
		Chat:        chat,
		From:        pointer(userFromAccount(sender)),
		Text:        plainText(msg),
		Caption:     messageCaption(msg),
		ReplyMarkup: messageReplyMarkup(msg.Metadata),
	}
	if msg.ThreadID != "" {
		result.MessageThreadID = msg.ThreadID
	}
	if !msg.EditedAt.IsZero() {
		result.EditDate = msg.EditedAt.UTC().Unix()
	}
	result.Photo, result.Document, result.Video, result.Animation, result.Audio, result.VideoNote, result.Voice, result.Sticker = messageMedia(msg)
	if !includeReply || msg.ReplyTo.MessageID == "" {
		return result, nil
	}

	reply, err := s.conversations.GetMessage(ctx, conversation.GetMessageParams{
		ConversationID: conv.ID,
		MessageID:      msg.ReplyTo.MessageID,
		AccountID:      botAccountID,
	})
	if err != nil {
		return result, nil
	}

	replyMessage, err := s.messageForConversation(ctx, botAccountID, conv, members, reply, false)
	if err != nil {
		return result, nil
	}
	result.ReplyToMessage = &replyMessage

	return result, nil
}

func pointer[T any](value T) *T {
	return &value
}
