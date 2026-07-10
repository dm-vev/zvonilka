package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// CallbackQuery returns a callback state visible to the account that pressed it.
func (s *Service) CallbackQuery(ctx context.Context, callbackID, accountID string) (Callback, error) {
	if err := s.validateContext(ctx, "load callback query"); err != nil {
		return Callback{}, err
	}
	callbackID = strings.TrimSpace(callbackID)
	accountID = strings.TrimSpace(accountID)
	if callbackID == "" || accountID == "" {
		return Callback{}, ErrInvalidInput
	}
	callback, err := s.store.CallbackByID(ctx, callbackID)
	if err != nil {
		return Callback{}, fmt.Errorf("load callback query %s: %w", callbackID, err)
	}
	if callback.FromAccountID != accountID {
		return Callback{}, ErrForbidden
	}
	return callback, nil
}

// InlineQuery returns an inline query state visible to the account that sent it.
func (s *Service) InlineQuery(ctx context.Context, queryID, accountID string) (InlineQueryState, error) {
	if err := s.validateContext(ctx, "load inline query"); err != nil {
		return InlineQueryState{}, err
	}
	queryID = strings.TrimSpace(queryID)
	accountID = strings.TrimSpace(accountID)
	if queryID == "" || accountID == "" {
		return InlineQueryState{}, ErrInvalidInput
	}
	query, err := s.store.InlineQueryByID(ctx, queryID)
	if err != nil {
		return InlineQueryState{}, fmt.Errorf("load inline query %s: %w", queryID, err)
	}
	if query.FromAccountID != accountID {
		return InlineQueryState{}, ErrForbidden
	}
	return query, nil
}

// SendInlineResult sends a selected inline result as the bot that answered it.
func (s *Service) SendInlineResult(
	ctx context.Context,
	queryID string,
	resultID string,
	accountID string,
	conversationID string,
	replyMessageID string,
) (conversation.Message, error) {
	query, err := s.InlineQuery(ctx, queryID, accountID)
	if err != nil {
		return conversation.Message{}, err
	}
	if !query.Answered {
		return conversation.Message{}, ErrConflict
	}
	resultID = strings.TrimSpace(resultID)
	conversationID = strings.TrimSpace(conversationID)
	if resultID == "" || conversationID == "" {
		return conversation.Message{}, ErrInvalidInput
	}
	var selected *InlineQueryResult
	for index := range query.Results {
		if query.Results[index].ID == resultID {
			selected = &query.Results[index]
			break
		}
	}
	if selected == nil {
		return conversation.Message{}, ErrNotFound
	}
	text := strings.TrimSpace(selected.Title)
	var textEntities []TextEntity
	if selected.InputMessageContent != nil {
		text = selected.InputMessageContent.MessageText
		textEntities = selected.InputMessageContent.Entities
	}
	if text == "" {
		text = selected.Caption
		textEntities = selected.CaptionEntities
	}
	if strings.TrimSpace(text) == "" {
		return conversation.Message{}, ErrInvalidInput
	}
	metadataValues := map[string]string{}
	mediaURL, mediaKind, mediaMIME := inlineMediaMetadata(*selected)
	if mediaURL != "" {
		metadataValues["bot.inline_media_url"] = mediaURL
		metadataValues["bot.inline_media_kind"] = mediaKind
		metadataValues["bot.inline_media_mime"] = mediaMIME
	}
	metadata, err := withTextEntities(metadataValues, metadataTextEntitiesKey, textEntities)
	if err != nil {
		return conversation.Message{}, err
	}
	metadata, err = markupMetadata(metadata, selected.ReplyMarkup)
	if err != nil {
		return conversation.Message{}, err
	}
	draft := conversation.MessageDraft{
		Kind: conversation.MessageKindText,
		Payload: conversation.EncryptedPayload{
			Ciphertext: []byte(text),
		},
		Metadata: metadata,
	}
	var message conversation.Message
	if replyMessageID = strings.TrimSpace(replyMessageID); replyMessageID != "" {
		message, _, err = s.conversations.ReplyMessage(ctx, conversation.ReplyMessageParams{
			ConversationID:   conversationID,
			SenderAccountID:  query.BotAccountID,
			SenderDeviceID:   botDeviceID,
			ReplyToMessageID: replyMessageID,
			Draft:            draft,
		})
	} else {
		message, _, err = s.conversations.SendMessage(ctx, conversation.SendMessageParams{
			ConversationID:  conversationID,
			SenderAccountID: query.BotAccountID,
			SenderDeviceID:  botDeviceID,
			Draft:           draft,
		})
	}
	if err != nil {
		return conversation.Message{}, fmt.Errorf("send inline result: %w", err)
	}
	if _, err := s.TriggerChosenInlineResult(ctx, TriggerChosenInlineResultParams{
		InlineQueryID: query.ID,
		FromAccountID: query.FromAccountID,
		ResultID:      resultID,
	}); err != nil {
		return conversation.Message{}, fmt.Errorf("record chosen inline result: %w", err)
	}
	return message, nil
}

func inlineMediaMetadata(result InlineQueryResult) (string, string, string) {
	switch result.Type {
	case "photo":
		return result.PhotoURL, "1", "image/jpeg"
	case "gif":
		return result.GIFURL, "6", "image/gif"
	case "mpeg4_gif":
		return result.Mpeg4URL, "6", "video/mp4"
	case "video":
		return result.VideoURL, "2", result.MimeType
	case "audio":
		return result.AudioURL, "3", result.MimeType
	case "document":
		return result.DocumentURL, "3", result.MimeType
	default:
		return "", "", ""
	}
}

// CommandsForAccount returns the default command set for an active bot account.
func (s *Service) CommandsForAccount(ctx context.Context, botAccountID, languageCode string) ([]Command, error) {
	if err := s.validateContext(ctx, "load bot commands"); err != nil {
		return nil, err
	}
	botAccountID = strings.TrimSpace(botAccountID)
	if botAccountID == "" {
		return nil, ErrInvalidInput
	}
	account, err := s.identity.AccountByID(ctx, botAccountID)
	if err != nil {
		return nil, fmt.Errorf("load bot account %s: %w", botAccountID, err)
	}
	if account.Kind != identity.AccountKindBot || account.Status != identity.AccountStatusActive {
		return nil, ErrForbidden
	}
	scope, normalizedLanguage, err := normalizeCommandLookup(CommandScope{Type: CommandScopeDefault}, languageCode)
	if err != nil {
		return nil, err
	}
	set, err := s.store.CommandsByScope(ctx, botAccountID, scope, normalizedLanguage)
	if err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("load bot commands %s: %w", botAccountID, err)
	}
	return append([]Command(nil), set.Commands...), nil
}
