package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
)

// SendLocation sends one location message as the authenticated bot.
func (s *Service) SendLocation(ctx context.Context, params SendLocationParams) (Message, error) {
	location := Location{
		Latitude:             params.Latitude,
		Longitude:            params.Longitude,
		HorizontalAccuracy:   params.HorizontalAccuracy,
		LivePeriod:           params.LivePeriod,
		Heading:              params.Heading,
		ProximityAlertRadius: params.ProximityAlertRadius,
	}
	if !validCoordinate(location.Latitude, -90, 90) || !validCoordinate(location.Longitude, -180, 180) {
		return Message{}, ErrInvalidInput
	}
	if location.HorizontalAccuracy < 0 || location.LivePeriod < 0 || location.Heading < 0 || location.ProximityAlertRadius < 0 {
		return Message{}, ErrInvalidInput
	}

	return s.sendStructured(ctx, structuredParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		ReplyToMessageID:    params.ReplyToMessageID,
		ReplyMarkup:         params.ReplyMarkup,
		DisableNotification: params.DisableNotification,
		Shape:               "location",
		Text:                fmt.Sprintf("%.6f,%.6f", location.Latitude, location.Longitude),
		Value:               location,
	})
}

// SendContact sends one contact message as the authenticated bot.
func (s *Service) SendContact(ctx context.Context, params SendContactParams) (Message, error) {
	contact := Contact{
		PhoneNumber: strings.TrimSpace(params.PhoneNumber),
		FirstName:   strings.TrimSpace(params.FirstName),
		LastName:    strings.TrimSpace(params.LastName),
		VCard:       strings.TrimSpace(params.VCard),
		UserID:      strings.TrimSpace(params.UserID),
	}
	if contact.PhoneNumber == "" || contact.FirstName == "" {
		return Message{}, ErrInvalidInput
	}

	text := strings.TrimSpace(strings.Join([]string{contact.FirstName, contact.LastName}, " "))
	if text == "" {
		text = contact.PhoneNumber
	}

	return s.sendStructured(ctx, structuredParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		ReplyToMessageID:    params.ReplyToMessageID,
		ReplyMarkup:         params.ReplyMarkup,
		DisableNotification: params.DisableNotification,
		Shape:               "contact",
		Text:                text,
		Value:               contact,
	})
}

// SendPoll sends one poll message as the authenticated bot.
func (s *Service) SendPoll(ctx context.Context, params SendPollParams) (Message, error) {
	question := strings.TrimSpace(params.Question)
	if question == "" || len(params.Options) < 2 {
		return Message{}, ErrInvalidInput
	}

	options := make([]PollOption, 0, len(params.Options))
	for _, option := range params.Options {
		option = strings.TrimSpace(option)
		if option == "" {
			return Message{}, ErrInvalidInput
		}
		options = append(options, PollOption{Text: option})
	}

	pollType := strings.TrimSpace(params.Type)
	if pollType == "" {
		pollType = "regular"
	}
	if pollType != "regular" && pollType != "quiz" {
		return Message{}, ErrInvalidInput
	}

	pollID, err := newID("poll")
	if err != nil {
		return Message{}, fmt.Errorf("generate poll id: %w", err)
	}
	poll := Poll{
		ID:                    pollID,
		Question:              question,
		Options:               options,
		IsAnonymous:           params.IsAnonymous,
		Type:                  pollType,
		AllowsMultipleAnswers: params.AllowsMultipleAnswers,
	}

	return s.sendStructured(ctx, structuredParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		ReplyToMessageID:    params.ReplyToMessageID,
		ReplyMarkup:         params.ReplyMarkup,
		DisableNotification: params.DisableNotification,
		Shape:               "poll",
		Text:                question,
		Value:               poll,
	})
}

type structuredParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
	Shape               string
	Text                string
	Value               any
}

// EditLiveLocation edits one live location message as the authenticated bot.
func (s *Service) EditLiveLocation(ctx context.Context, params EditLiveLocationParams) (Message, error) {
	accountID, conv, _, raw, err := s.loadRawMessage(ctx, params.BotToken, params.ChatID, params.MessageID)
	if err != nil {
		return Message{}, err
	}
	if messageShape(raw) != "location" {
		return Message{}, ErrInvalidInput
	}

	location := Location{
		Latitude:             params.Latitude,
		Longitude:            params.Longitude,
		HorizontalAccuracy:   params.HorizontalAccuracy,
		LivePeriod:           params.LivePeriod,
		Heading:              params.Heading,
		ProximityAlertRadius: params.ProximityAlertRadius,
	}
	if !validCoordinate(location.Latitude, -90, 90) || !validCoordinate(location.Longitude, -180, 180) {
		return Message{}, ErrInvalidInput
	}
	if location.HorizontalAccuracy < 0 || location.LivePeriod < 0 || location.Heading < 0 || location.ProximityAlertRadius < 0 {
		return Message{}, ErrInvalidInput
	}

	replyMarkup := params.ReplyMarkup
	if replyMarkup == nil {
		replyMarkup = messageReplyMarkup(raw.Metadata)
	}

	payload, err := json.Marshal(location)
	if err != nil {
		return Message{}, fmt.Errorf("marshal edited live location: %w", err)
	}
	metadata, err := markupMetadata(map[string]string{
		metadataShapeKey: "location",
		metadataJSONKey:  string(payload),
	}, replyMarkup)
	if err != nil {
		return Message{}, err
	}

	message, _, err := s.conversations.EditMessage(ctx, conversation.EditMessageParams{
		ConversationID: conv.ID,
		MessageID:      raw.ID,
		ActorAccountID: accountID,
		ActorDeviceID:  botDeviceID,
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte(fmt.Sprintf("%.6f,%.6f", location.Latitude, location.Longitude)),
			},
			ThreadID: raw.ThreadID,
			Silent:   raw.Silent,
			Metadata: metadata,
		},
	})
	if err != nil {
		return Message{}, fmt.Errorf("edit bot live location: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    conv.ID,
		MessageID: message.ID,
	})
}

// StopPoll closes one poll and returns the final poll state.
func (s *Service) StopPoll(ctx context.Context, params StopPollParams) (Poll, error) {
	accountID, conv, _, raw, err := s.loadRawMessage(ctx, params.BotToken, params.ChatID, params.MessageID)
	if err != nil {
		return Poll{}, err
	}
	if messageShape(raw) != "poll" {
		return Poll{}, ErrInvalidInput
	}

	poll := messagePoll(raw)
	if poll == nil {
		return Poll{}, ErrInvalidInput
	}
	poll.IsClosed = true

	replyMarkup := params.ReplyMarkup
	if replyMarkup == nil {
		replyMarkup = messageReplyMarkup(raw.Metadata)
	}

	payload, err := json.Marshal(poll)
	if err != nil {
		return Poll{}, fmt.Errorf("marshal stopped poll: %w", err)
	}
	metadata, err := markupMetadata(map[string]string{
		metadataShapeKey: "poll",
		metadataJSONKey:  string(payload),
	}, replyMarkup)
	if err != nil {
		return Poll{}, err
	}

	_, _, err = s.conversations.EditMessage(ctx, conversation.EditMessageParams{
		ConversationID: conv.ID,
		MessageID:      raw.ID,
		ActorAccountID: accountID,
		ActorDeviceID:  botDeviceID,
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte(strings.TrimSpace(poll.Question)),
			},
			ThreadID: raw.ThreadID,
			Silent:   raw.Silent,
			Metadata: metadata,
		},
	})
	if err != nil {
		return Poll{}, fmt.Errorf("stop bot poll: %w", mapConversationError(err))
	}

	return *poll, nil
}

func (s *Service) sendStructured(ctx context.Context, params structuredParams) (Message, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return Message{}, err
	}

	params.ChatID = strings.TrimSpace(params.ChatID)
	params.MessageThreadID = strings.TrimSpace(params.MessageThreadID)
	params.ReplyToMessageID = strings.TrimSpace(params.ReplyToMessageID)
	params.Shape = strings.TrimSpace(params.Shape)
	params.Text = strings.TrimSpace(params.Text)
	if params.ChatID == "" || params.Shape == "" || params.Text == "" || params.Value == nil {
		return Message{}, ErrInvalidInput
	}

	payload, err := json.Marshal(params.Value)
	if err != nil {
		return Message{}, fmt.Errorf("marshal structured bot payload: %w", err)
	}
	metadata, err := markupMetadata(map[string]string{
		metadataShapeKey: params.Shape,
		metadataJSONKey:  string(payload),
	}, params.ReplyMarkup)
	if err != nil {
		return Message{}, err
	}

	draft := conversation.MessageDraft{
		Kind: conversation.MessageKindText,
		Payload: conversation.EncryptedPayload{
			Ciphertext: []byte(params.Text),
		},
		ThreadID: params.MessageThreadID,
		Silent:   params.DisableNotification,
		Metadata: metadata,
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
		return Message{}, fmt.Errorf("send bot structured message: %w", mapConversationError(err))
	}

	return s.GetMessage(ctx, GetMessageParams{
		BotToken:  params.BotToken,
		ChatID:    params.ChatID,
		MessageID: message.ID,
	})
}

func validCoordinate(value float64, min float64, max float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= min && value <= max
}
