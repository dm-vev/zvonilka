package gateway

import (
	"context"
	"encoding/json"

	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// TriggerCallbackQuery creates a callback update for the bot that owns a message.
func (a *api) TriggerCallbackQuery(
	ctx context.Context,
	req *conversationv1.TriggerCallbackQueryRequest,
) (*conversationv1.TriggerCallbackQueryResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if a.bot == nil || req == nil {
		return nil, grpcError(domainbot.ErrInvalidInput)
	}
	callback, err := a.bot.TriggerCallbackQuery(ctx, domainbot.TriggerCallbackParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		FromAccountID:  authContext.Account.ID,
		Data:           req.GetData(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.TriggerCallbackQueryResponse{CallbackQueryId: callback.ID}, nil
}

// GetCallbackQueryAnswer returns the current answer for a callback query.
func (a *api) GetCallbackQueryAnswer(
	ctx context.Context,
	req *conversationv1.GetCallbackQueryAnswerRequest,
) (*conversationv1.GetCallbackQueryAnswerResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if a.bot == nil || req == nil {
		return nil, grpcError(domainbot.ErrInvalidInput)
	}
	callback, err := a.bot.CallbackQuery(ctx, req.GetCallbackQueryId(), authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.GetCallbackQueryAnswerResponse{
		Answered:  !callback.AnsweredAt.IsZero(),
		Text:      callback.AnsweredText,
		Url:       callback.AnsweredURL,
		ShowAlert: callback.ShowAlert,
		CacheTime: uint32(callback.CacheTime),
	}, nil
}

// TriggerInlineQuery creates an inline query update for a bot.
func (a *api) TriggerInlineQuery(
	ctx context.Context,
	req *conversationv1.TriggerInlineQueryRequest,
) (*conversationv1.TriggerInlineQueryResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if a.bot == nil || req == nil {
		return nil, grpcError(domainbot.ErrInvalidInput)
	}
	query, err := a.bot.TriggerInlineQuery(ctx, domainbot.TriggerInlineQueryParams{
		BotAccountID:  req.GetBotUserId(),
		FromAccountID: authContext.Account.ID,
		Query:         req.GetQuery(),
		Offset:        req.GetOffset(),
		ChatType:      req.GetChatType(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.TriggerInlineQueryResponse{InlineQueryId: query.ID}, nil
}

// GetInlineQueryAnswer returns the current answer for an inline query.
func (a *api) GetInlineQueryAnswer(
	ctx context.Context,
	req *conversationv1.GetInlineQueryAnswerRequest,
) (*conversationv1.GetInlineQueryAnswerResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if a.bot == nil || req == nil {
		return nil, grpcError(domainbot.ErrInvalidInput)
	}
	query, err := a.bot.InlineQuery(ctx, req.GetInlineQueryId(), authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	results := make([]*conversationv1.InlineQueryResult, 0, len(query.Results))
	for _, result := range query.Results {
		results = append(results, &conversationv1.InlineQueryResult{
			Type:            result.Type,
			Id:              result.ID,
			Title:           result.Title,
			Description:     result.Description,
			Caption:         result.Caption,
			MessageText:     inlineMessageText(result),
			ReplyMarkupJson: inlineReplyMarkupJSON(result),
			PhotoUrl:        result.PhotoURL,
			AudioUrl:        result.AudioURL,
			DocumentUrl:     result.DocumentURL,
			GifUrl:          result.GIFURL,
			Mpeg4Url:        result.Mpeg4URL,
			VideoUrl:        result.VideoURL,
			MimeType:        result.MimeType,
			ThumbUrl:        result.ThumbURL,
		})
	}
	return &conversationv1.GetInlineQueryAnswerResponse{
		Answered:          query.Answered,
		Results:           results,
		NextOffset:        query.NextOffset,
		SwitchPmText:      query.SwitchPMText,
		SwitchPmParameter: query.SwitchPMParam,
	}, nil
}

// SendInlineQueryResult sends a selected bot result into a conversation.
func (a *api) SendInlineQueryResult(
	ctx context.Context,
	req *conversationv1.SendInlineQueryResultRequest,
) (*conversationv1.SendInlineQueryResultResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if a.bot == nil || req == nil {
		return nil, grpcError(domainbot.ErrInvalidInput)
	}
	message, err := a.bot.SendInlineResult(
		ctx,
		req.GetInlineQueryId(),
		req.GetResultId(),
		authContext.Account.ID,
		req.GetConversationId(),
		req.GetReplyMessageId(),
	)
	if err != nil {
		return nil, grpcError(err)
	}
	profiles, err := a.messageProfiles(ctx, []domainconversation.Message{message}, authContext.Account.ID)
	if err != nil {
		return nil, grpcError(err)
	}
	return &conversationv1.SendInlineQueryResultResponse{
		Message: messageProto(message, profiles[message.SenderAccountID], nil),
	}, nil
}

// GetBotCommands returns the bot's default command set for the current locale.
func (a *api) GetBotCommands(
	ctx context.Context,
	req *conversationv1.GetBotCommandsRequest,
) (*conversationv1.GetBotCommandsResponse, error) {
	if _, err := a.requireAuth(ctx); err != nil {
		return nil, err
	}
	if a.bot == nil || req == nil {
		return nil, grpcError(domainbot.ErrInvalidInput)
	}
	commands, err := a.bot.CommandsForAccount(ctx, req.GetBotUserId(), req.GetLanguageCode())
	if err != nil {
		return nil, grpcError(err)
	}
	result := make([]*conversationv1.BotCommand, 0, len(commands))
	for _, command := range commands {
		result = append(result, &conversationv1.BotCommand{
			Command:     command.Command,
			Description: command.Description,
		})
	}
	return &conversationv1.GetBotCommandsResponse{Commands: result}, nil
}

func inlineMessageText(result domainbot.InlineQueryResult) string {
	if result.InputMessageContent == nil {
		return ""
	}
	return result.InputMessageContent.MessageText
}

func inlineReplyMarkupJSON(result domainbot.InlineQueryResult) string {
	if result.ReplyMarkup == nil {
		return ""
	}
	encoded, err := json.Marshal(result.ReplyMarkup)
	if err != nil {
		return ""
	}
	return string(encoded)
}
