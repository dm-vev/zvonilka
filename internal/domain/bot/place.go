package bot

import (
	"context"
	"fmt"
	"strings"
)

// Venue describes one Telegram-shaped venue projection.
type Venue struct {
	Location        Location `json:"location"`
	Title           string   `json:"title"`
	Address         string   `json:"address"`
	FoursquareID    string   `json:"foursquare_id,omitempty"`
	FoursquareType  string   `json:"foursquare_type,omitempty"`
	GooglePlaceID   string   `json:"google_place_id,omitempty"`
	GooglePlaceType string   `json:"google_place_type,omitempty"`
}

// Dice describes one Telegram-shaped dice projection.
type Dice struct {
	Emoji string `json:"emoji"`
	Value int    `json:"value"`
}

// SendVenueParams describes one sendVenue request.
type SendVenueParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	Latitude            float64
	Longitude           float64
	Title               string
	Address             string
	FoursquareID        string
	FoursquareType      string
	GooglePlaceID       string
	GooglePlaceType     string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendDiceParams describes one sendDice request.
type SendDiceParams struct {
	BotToken            string
	ChatID              string
	MessageThreadID     string
	Emoji               string
	ReplyToMessageID    string
	ReplyMarkup         *InlineKeyboardMarkup
	DisableNotification bool
}

// SendVenue sends one venue message as the authenticated bot.
func (s *Service) SendVenue(ctx context.Context, params SendVenueParams) (Message, error) {
	venue := Venue{
		Location: Location{
			Latitude:  params.Latitude,
			Longitude: params.Longitude,
		},
		Title:           strings.TrimSpace(params.Title),
		Address:         strings.TrimSpace(params.Address),
		FoursquareID:    strings.TrimSpace(params.FoursquareID),
		FoursquareType:  strings.TrimSpace(params.FoursquareType),
		GooglePlaceID:   strings.TrimSpace(params.GooglePlaceID),
		GooglePlaceType: strings.TrimSpace(params.GooglePlaceType),
	}
	if !validCoordinate(venue.Location.Latitude, -90, 90) ||
		!validCoordinate(venue.Location.Longitude, -180, 180) ||
		venue.Title == "" ||
		venue.Address == "" {
		return Message{}, ErrInvalidInput
	}

	return s.sendStructured(ctx, structuredParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		ReplyToMessageID:    params.ReplyToMessageID,
		ReplyMarkup:         params.ReplyMarkup,
		DisableNotification: params.DisableNotification,
		Shape:               "venue",
		Text:                fmt.Sprintf("%s %s", venue.Title, venue.Address),
		Value:               venue,
	})
}

// SendDice sends one dice message as the authenticated bot.
func (s *Service) SendDice(ctx context.Context, params SendDiceParams) (Message, error) {
	emoji := strings.TrimSpace(params.Emoji)
	if emoji == "" {
		emoji = "🎲"
	}

	return s.sendStructured(ctx, structuredParams{
		BotToken:            params.BotToken,
		ChatID:              params.ChatID,
		MessageThreadID:     params.MessageThreadID,
		ReplyToMessageID:    params.ReplyToMessageID,
		ReplyMarkup:         params.ReplyMarkup,
		DisableNotification: params.DisableNotification,
		Shape:               "dice",
		Text:                emoji,
		Value: Dice{
			Emoji: emoji,
			Value: 1,
		},
	})
}
