package conversation

import (
	"context"
	"strings"
)

// ReplyMessage sends a message as a reply to an existing message.
func (s *Service) ReplyMessage(ctx context.Context, params ReplyMessageParams) (Message, EventEnvelope, error) {
	params.ConversationID = strings.TrimSpace(params.ConversationID)
	params.SenderAccountID = strings.TrimSpace(params.SenderAccountID)
	params.SenderDeviceID = strings.TrimSpace(params.SenderDeviceID)
	params.ReplyToMessageID = strings.TrimSpace(params.ReplyToMessageID)
	if params.ReplyToMessageID == "" {
		return Message{}, EventEnvelope{}, ErrInvalidInput
	}

	params.Draft.ReplyTo = MessageReference{
		MessageID: params.ReplyToMessageID,
	}

	return s.SendMessage(ctx, SendMessageParams{
		ConversationID:  params.ConversationID,
		SenderAccountID: params.SenderAccountID,
		SenderDeviceID:  params.SenderDeviceID,
		Draft:           params.Draft,
		CausationID:     params.CausationID,
		CorrelationID:   params.CorrelationID,
		CreatedAt:       params.CreatedAt,
	})
}
