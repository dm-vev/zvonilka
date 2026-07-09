package bot

import (
	"context"
	"strings"
)

// SendGame sends one Telegram-shaped game message as the authenticated bot.
func (s *Service) SendGame(ctx context.Context, params SendGameParams) (Message, error) {
	shortName := strings.TrimSpace(params.GameShortName)
	if shortName == "" {
		return Message{}, ErrInvalidInput
	}

	game := Game{
		Title:       shortName,
		Description: shortName,
		Photo:       []PhotoSize{},
		Text:        shortName,
		ShortName:   shortName,
	}

	return s.sendStructured(ctx, structuredParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		ReplyToMessageID:    params.ReplyToMessageID,
		ReplyMarkup:         params.ReplyMarkup,
		DisableNotification: params.DisableNotification,
		Shape:               "game",
		Text:                shortName,
		Value:               game,
	})
}
