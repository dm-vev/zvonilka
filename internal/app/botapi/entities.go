package botapi

import (
	"context"
	"fmt"
	"strconv"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	tgmodels "github.com/go-telegram/bot/models"
)

func (a *api) domainTextEntities(
	ctx context.Context,
	values []tgmodels.MessageEntity,
) ([]domainbot.TextEntity, error) {
	if len(values) == 0 {
		return nil, nil
	}

	result := make([]domainbot.TextEntity, 0, len(values))
	for _, value := range values {
		entity := domainbot.TextEntity{
			Type:          string(value.Type),
			Offset:        value.Offset,
			Length:        value.Length,
			URL:           value.URL,
			Language:      value.Language,
			CustomEmojiID: value.CustomEmojiID,
		}
		if value.User != nil {
			userID, err := a.internalUserID(ctx, textID(strconv.FormatInt(value.User.ID, 10)))
			if err != nil {
				return nil, fmt.Errorf("resolve text entity user %d: %w", value.User.ID, err)
			}
			entity.UserID = userID
		}
		result = append(result, entity)
	}

	return result, nil
}

func (a *api) formatText(
	ctx context.Context,
	text string,
	parseMode string,
	values []tgmodels.MessageEntity,
) (string, []domainbot.TextEntity, error) {
	entities, err := a.domainTextEntities(ctx, values)
	if err != nil {
		return "", nil, err
	}
	return domainbot.FormatText(text, parseMode, entities)
}

func (a *api) telegramTextEntities(
	ctx context.Context,
	values []domainbot.TextEntity,
) ([]tgmodels.MessageEntity, error) {
	if len(values) == 0 {
		return nil, nil
	}

	result := make([]tgmodels.MessageEntity, 0, len(values))
	for _, value := range values {
		entity := tgmodels.MessageEntity{
			Type:          tgmodels.MessageEntityType(value.Type),
			Offset:        value.Offset,
			Length:        value.Length,
			URL:           value.URL,
			Language:      value.Language,
			CustomEmojiID: value.CustomEmojiID,
		}
		if value.UserID != "" {
			publicID, err := a.bot.PublicAccountID(ctx, value.UserID)
			if err != nil {
				return nil, fmt.Errorf("resolve text entity public user %s: %w", value.UserID, err)
			}
			entity.User = &tgmodels.User{ID: publicID}
		}
		result = append(result, entity)
	}

	return result, nil
}
