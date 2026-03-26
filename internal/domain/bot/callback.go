package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// TriggerCallbackQuery enqueues one callback query update for a bot-owned message.
func (s *Service) TriggerCallbackQuery(ctx context.Context, params TriggerCallbackParams) (CallbackQuery, error) {
	if err := s.validateContext(ctx, "trigger callback query"); err != nil {
		return CallbackQuery{}, err
	}

	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.MessageID = strings.TrimSpace(params.MessageID)
	params.FromAccountID = strings.TrimSpace(params.FromAccountID)
	params.Data = strings.TrimSpace(params.Data)
	if params.ConversationID == "" || params.MessageID == "" || params.FromAccountID == "" || params.Data == "" {
		return CallbackQuery{}, ErrInvalidInput
	}

	conv, members, message, actor, botAccount, err := s.callbackContext(ctx, params.ConversationID, params.MessageID, params.FromAccountID)
	if err != nil {
		return CallbackQuery{}, err
	}

	markup := messageReplyMarkup(message.Metadata)
	if !callbackAllowed(markup, params.Data) {
		return CallbackQuery{}, ErrForbidden
	}

	callbackID, err := newID("cbq")
	if err != nil {
		return CallbackQuery{}, err
	}
	now := s.currentTime()
	callback := Callback{
		ID:              callbackID,
		BotAccountID:    botAccount.ID,
		FromAccountID:   actor.ID,
		ConversationID:  conv.ID,
		MessageID:       message.ID,
		MessageThreadID: message.ThreadID,
		ChatInstance:    conv.ID,
		Data:            params.Data,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	projectedMessage, err := s.messageForConversation(ctx, botAccount.ID, conv, members, message, true)
	if err != nil {
		return CallbackQuery{}, err
	}
	updatePayload := Update{
		CallbackQuery: &CallbackQuery{
			ID:           callback.ID,
			From:         userFromAccount(actor),
			Message:      &projectedMessage,
			ChatInstance: callback.ChatInstance,
			Data:         callback.Data,
		},
	}

	err = s.runTx(ctx, func(tx Store) error {
		if _, err := tx.SaveCallback(ctx, callback); err != nil {
			return fmt.Errorf("save callback query %s: %w", callback.ID, err)
		}
		if _, err := tx.SaveUpdate(ctx, QueueEntry{
			BotAccountID:  botAccount.ID,
			EventID:       callback.ID,
			UpdateType:    UpdateTypeCallbackQuery,
			Payload:       updatePayload,
			NextAttemptAt: now,
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			return fmt.Errorf("save callback query update %s: %w", callback.ID, err)
		}

		return nil
	})
	if err != nil {
		return CallbackQuery{}, err
	}

	return *updatePayload.CallbackQuery, nil
}

// AnswerCallbackQuery acknowledges one pending callback query.
func (s *Service) AnswerCallbackQuery(ctx context.Context, params AnswerCallbackQueryParams) error {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return err
	}

	params.CallbackQueryID = strings.TrimSpace(params.CallbackQueryID)
	params.Text = strings.TrimSpace(params.Text)
	params.URL = strings.TrimSpace(params.URL)
	if params.CallbackQueryID == "" {
		return ErrInvalidInput
	}

	callback, err := s.store.CallbackByID(ctx, params.CallbackQueryID)
	if err != nil {
		return fmt.Errorf("load callback query %s: %w", params.CallbackQueryID, err)
	}
	if callback.BotAccountID != account.ID {
		return ErrForbidden
	}

	now := s.currentTime()
	callback.AnsweredText = params.Text
	callback.AnsweredURL = params.URL
	callback.ShowAlert = params.ShowAlert
	callback.CacheTime = params.CacheTimeSeconds
	callback.AnsweredAt = now
	callback.UpdatedAt = now

	if _, err := s.store.AnswerCallback(ctx, callback); err != nil {
		return fmt.Errorf("answer callback query %s: %w", callback.ID, err)
	}

	return nil
}

func (s *Service) callbackContext(
	ctx context.Context,
	conversationID string,
	messageID string,
	fromAccountID string,
) (conversation.Conversation, []conversation.ConversationMember, conversation.Message, identity.Account, identity.Account, error) {
	conv, err := s.conversationDB.ConversationByID(ctx, conversationID)
	if err != nil {
		return conversation.Conversation{}, nil, conversation.Message{}, identity.Account{}, identity.Account{}, mapConversationError(err)
	}
	message, err := s.conversations.GetMessage(ctx, conversation.GetMessageParams{
		ConversationID: conversationID,
		MessageID:      messageID,
		AccountID:      fromAccountID,
	})
	if err != nil {
		return conversation.Conversation{}, nil, conversation.Message{}, identity.Account{}, identity.Account{}, mapConversationError(err)
	}
	members, err := s.conversationDB.ConversationMembersByConversationID(ctx, conversationID)
	if err != nil {
		return conversation.Conversation{}, nil, conversation.Message{}, identity.Account{}, identity.Account{}, mapConversationError(err)
	}
	actor, err := s.identity.AccountByID(ctx, fromAccountID)
	if err != nil {
		return conversation.Conversation{}, nil, conversation.Message{}, identity.Account{}, identity.Account{}, mapIdentityError(err)
	}
	botAccount, err := s.identity.AccountByID(ctx, message.SenderAccountID)
	if err != nil {
		return conversation.Conversation{}, nil, conversation.Message{}, identity.Account{}, identity.Account{}, mapIdentityError(err)
	}
	if botAccount.Kind != identity.AccountKindBot || botAccount.Status != identity.AccountStatusActive {
		return conversation.Conversation{}, nil, conversation.Message{}, identity.Account{}, identity.Account{}, ErrForbidden
	}

	return conv, members, message, actor, botAccount, nil
}
