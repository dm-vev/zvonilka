package gateway

import (
	"context"

	conversationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/conversation/v1"
	domaintranslation "github.com/dm-vev/zvonilka/internal/domain/translation"
)

// TranslateMessage translates one visible text message into the requested language.
func (a *api) TranslateMessage(
	ctx context.Context,
	req *conversationv1.TranslateMessageRequest,
) (*conversationv1.TranslateMessageResponse, error) {
	authContext, err := a.requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	if !a.features.TranslationEnabled || a.translation == nil {
		return nil, featureDisabledError("translation")
	}

	translationRow, cached, err := a.translation.TranslateMessage(ctx, domaintranslation.TranslateMessageParams{
		ConversationID: req.GetConversationId(),
		MessageID:      req.GetMessageId(),
		RequesterID:    authContext.Account.ID,
		TargetLanguage: req.GetTargetLanguage(),
		SourceLanguage: req.GetSourceLanguage(),
		ForceRefresh:   req.GetForceRefresh(),
	})
	if err != nil {
		return nil, grpcError(err)
	}

	return &conversationv1.TranslateMessageResponse{
		Translation: &conversationv1.MessageTranslation{
			ConversationId: req.GetConversationId(),
			MessageId:      translationRow.MessageID,
			TargetLanguage: translationRow.TargetLanguage,
			SourceLanguage: translationRow.SourceLanguage,
			Provider:       translationRow.Provider,
			TranslatedText: translationRow.TranslatedText,
			Cached:         cached,
			TranslatedAt:   protoTime(translationRow.UpdatedAt),
		},
	}, nil
}
