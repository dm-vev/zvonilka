package botapi

import (
	"context"
	"strconv"

	tgmodels "github.com/go-telegram/bot/models"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
)

func (a *api) telegramUser(ctx context.Context, value domainbot.User) (tgmodels.User, error) {
	publicID, err := a.bot.PublicAccountID(ctx, value.ID)
	if err != nil {
		return tgmodels.User{}, err
	}

	return tgmodels.User{
		ID:                      publicID,
		IsBot:                   value.IsBot,
		FirstName:               value.FirstName,
		Username:                value.Username,
		CanJoinGroups:           value.CanJoinGroups,
		CanReadAllGroupMessages: value.CanReadAllGroupMessages,
		SupportInlineQueries:    value.SupportsInlineQueries,
	}, nil
}

func (a *api) telegramChat(ctx context.Context, value domainbot.Chat) (tgmodels.Chat, error) {
	publicID, err := a.bot.PublicChatID(ctx, value.ID)
	if err != nil {
		return tgmodels.Chat{}, err
	}

	chat := tgmodels.Chat{
		ID:       publicID,
		Type:     tgmodels.ChatType(value.Type),
		Title:    value.Title,
		Username: value.Username,
		IsForum:  value.IsForum,
	}
	if value.Type == domainbot.ChatTypePrivate {
		chat.FirstName = value.Title
	}

	return chat, nil
}

func (a *api) telegramMessage(ctx context.Context, value domainbot.Message) (*tgmodels.Message, error) {
	messageID, err := a.bot.PublicMessageID(ctx, value.MessageID)
	if err != nil {
		return nil, err
	}
	chat, err := a.telegramChat(ctx, value.Chat)
	if err != nil {
		return nil, err
	}

	result := &tgmodels.Message{
		ID:             int(messageID),
		Date:           int(value.Date),
		Chat:           chat,
		Text:           value.Text,
		Caption:        value.Caption,
		Photo:          a.telegramPhotos(value.Photo),
		Document:       a.telegramDocument(value.Document),
		Video:          a.telegramVideo(value.Video),
		Animation:      a.telegramAnimation(value.Animation),
		Audio:          a.telegramAudio(value.Audio),
		VideoNote:      a.telegramVideoNote(value.VideoNote),
		Voice:          a.telegramVoice(value.Voice),
		Sticker:        a.telegramSticker(value.Sticker),
		Location:       a.telegramLocation(value.Location),
		Contact:        a.telegramContact(ctx, value.Contact),
		Poll:           a.telegramPoll(value.Poll),
		Game:           a.telegramGame(value.Game),
		Venue:          a.telegramVenue(value.Venue),
		Dice:           a.telegramDice(value.Dice),
		ReplyMarkup:    telegramReplyMarkup(value.ReplyMarkup),
		IsTopicMessage: value.MessageThreadID != "",
	}
	if value.EditDate > 0 {
		result.EditDate = int(value.EditDate)
	}
	if value.From != nil {
		from, err := a.telegramUser(ctx, *value.From)
		if err != nil {
			return nil, err
		}
		result.From = &from
	}
	if value.MessageThreadID != "" {
		threadID, err := a.bot.PublicTopicID(ctx, value.MessageThreadID)
		if err != nil {
			return nil, err
		}
		result.MessageThreadID = int(threadID)
	}
	if value.ReplyToMessage != nil {
		reply, err := a.telegramMessage(ctx, *value.ReplyToMessage)
		if err != nil {
			return nil, err
		}
		result.ReplyToMessage = reply
	}

	return result, nil
}

func (a *api) telegramUpdate(ctx context.Context, value domainbot.Update) (*tgmodels.Update, error) {
	result := &tgmodels.Update{ID: value.UpdateID}

	var err error
	if value.Message != nil {
		result.Message, err = a.telegramMessage(ctx, *value.Message)
		if err != nil {
			return nil, err
		}
	}
	if value.EditedMessage != nil {
		result.EditedMessage, err = a.telegramMessage(ctx, *value.EditedMessage)
		if err != nil {
			return nil, err
		}
	}
	if value.ChannelPost != nil {
		result.ChannelPost, err = a.telegramMessage(ctx, *value.ChannelPost)
		if err != nil {
			return nil, err
		}
	}
	if value.EditedChannelPost != nil {
		result.EditedChannelPost, err = a.telegramMessage(ctx, *value.EditedChannelPost)
		if err != nil {
			return nil, err
		}
	}
	if value.CallbackQuery != nil {
		query, err := a.telegramCallback(ctx, *value.CallbackQuery)
		if err != nil {
			return nil, err
		}
		result.CallbackQuery = query
	}
	if value.InlineQuery != nil {
		query, err := a.telegramInlineQuery(ctx, *value.InlineQuery)
		if err != nil {
			return nil, err
		}
		result.InlineQuery = query
	}
	if value.ChosenInlineResult != nil {
		chosen, err := a.telegramChosen(ctx, *value.ChosenInlineResult)
		if err != nil {
			return nil, err
		}
		result.ChosenInlineResult = chosen
	}
	if value.ChatMember != nil {
		member, err := a.telegramMemberUpdate(ctx, *value.ChatMember)
		if err != nil {
			return nil, err
		}
		result.ChatMember = member
	}
	if value.MyChatMember != nil {
		member, err := a.telegramMemberUpdate(ctx, *value.MyChatMember)
		if err != nil {
			return nil, err
		}
		result.MyChatMember = member
	}

	return result, nil
}

func (a *api) telegramUpdates(ctx context.Context, values []domainbot.Update) ([]tgmodels.Update, error) {
	result := make([]tgmodels.Update, 0, len(values))
	for _, value := range values {
		update, err := a.telegramUpdate(ctx, value)
		if err != nil {
			return nil, err
		}
		result = append(result, *update)
	}

	return result, nil
}

func (a *api) telegramCallback(ctx context.Context, value domainbot.CallbackQuery) (*tgmodels.CallbackQuery, error) {
	from, err := a.telegramUser(ctx, value.From)
	if err != nil {
		return nil, err
	}

	result := &tgmodels.CallbackQuery{
		ID:              value.ID,
		From:            from,
		InlineMessageID: value.InlineMessageID,
		ChatInstance:    value.ChatInstance,
		Data:            value.Data,
	}
	if value.Message != nil {
		message, err := a.telegramMessage(ctx, *value.Message)
		if err != nil {
			return nil, err
		}
		result.Message = tgmodels.MaybeInaccessibleMessage{
			Type:    tgmodels.MaybeInaccessibleMessageTypeMessage,
			Message: message,
		}
	}

	return result, nil
}

func (a *api) telegramInlineQuery(ctx context.Context, value domainbot.InlineQuery) (*tgmodels.InlineQuery, error) {
	from, err := a.telegramUser(ctx, value.From)
	if err != nil {
		return nil, err
	}

	return &tgmodels.InlineQuery{
		ID:       value.ID,
		From:     &from,
		Query:    value.Query,
		Offset:   value.Offset,
		ChatType: value.ChatType,
	}, nil
}

func (a *api) telegramChosen(ctx context.Context, value domainbot.ChosenInlineResult) (*tgmodels.ChosenInlineResult, error) {
	from, err := a.telegramUser(ctx, value.From)
	if err != nil {
		return nil, err
	}

	return &tgmodels.ChosenInlineResult{
		ResultID:        value.ResultID,
		From:            from,
		Query:           value.Query,
		InlineMessageID: value.InlineMessageID,
	}, nil
}

func (a *api) telegramMemberUpdate(
	ctx context.Context,
	value domainbot.ChatMemberUpdated,
) (*tgmodels.ChatMemberUpdated, error) {
	chat, err := a.telegramChat(ctx, value.Chat)
	if err != nil {
		return nil, err
	}
	from, err := a.telegramUser(ctx, value.From)
	if err != nil {
		return nil, err
	}
	oldMember, err := a.telegramMember(ctx, value.OldChatMember)
	if err != nil {
		return nil, err
	}
	newMember, err := a.telegramMember(ctx, value.NewChatMember)
	if err != nil {
		return nil, err
	}

	return &tgmodels.ChatMemberUpdated{
		Chat:          chat,
		From:          from,
		Date:          int(value.Date),
		OldChatMember: oldMember,
		NewChatMember: newMember,
	}, nil
}

func (a *api) telegramMember(ctx context.Context, value domainbot.ChatMember) (tgmodels.ChatMember, error) {
	user, err := a.telegramUser(ctx, value.User)
	if err != nil {
		return tgmodels.ChatMember{}, err
	}

	switch value.Status {
	case domainbot.MemberStatusCreator:
		return tgmodels.ChatMember{
			Type:  tgmodels.ChatMemberTypeOwner,
			Owner: &tgmodels.ChatMemberOwner{User: &user},
		}, nil
	case domainbot.MemberStatusAdministrator:
		return tgmodels.ChatMember{
			Type: tgmodels.ChatMemberTypeAdministrator,
			Administrator: &tgmodels.ChatMemberAdministrator{
				User:                user,
				CanManageChat:       true,
				CanDeleteMessages:   true,
				CanManageVideoChats: true,
				CanRestrictMembers:  true,
				CanPromoteMembers:   true,
				CanChangeInfo:       true,
				CanInviteUsers:      true,
				CanPinMessages:      true,
			},
		}, nil
	case domainbot.MemberStatusRestricted:
		return tgmodels.ChatMember{
			Type: tgmodels.ChatMemberTypeRestricted,
			Restricted: &tgmodels.ChatMemberRestricted{
				User:            &user,
				IsMember:        true,
				CanSendMessages: false,
			},
		}, nil
	case domainbot.MemberStatusLeft:
		return tgmodels.ChatMember{
			Type: tgmodels.ChatMemberTypeLeft,
			Left: &tgmodels.ChatMemberLeft{User: &user},
		}, nil
	case domainbot.MemberStatusKicked:
		return tgmodels.ChatMember{
			Type:   tgmodels.ChatMemberTypeBanned,
			Banned: &tgmodels.ChatMemberBanned{User: &user},
		}, nil
	default:
		return tgmodels.ChatMember{
			Type:   tgmodels.ChatMemberTypeMember,
			Member: &tgmodels.ChatMemberMember{User: &user},
		}, nil
	}
}

func (a *api) telegramPhotos(values []domainbot.PhotoSize) []tgmodels.PhotoSize {
	if len(values) == 0 {
		return nil
	}

	result := make([]tgmodels.PhotoSize, 0, len(values))
	for _, value := range values {
		result = append(result, tgmodels.PhotoSize{
			FileID:       value.FileID,
			FileUniqueID: value.FileUniqueID,
			FileSize:     int(value.FileSize),
			Width:        int(value.Width),
			Height:       int(value.Height),
		})
	}

	return result
}

func (a *api) telegramDocument(value *domainbot.Document) *tgmodels.Document {
	if value == nil {
		return nil
	}

	return &tgmodels.Document{
		FileID:       value.FileID,
		FileUniqueID: value.FileUniqueID,
		FileName:     value.FileName,
		MimeType:     value.MimeType,
		FileSize:     int64(value.FileSize),
	}
}

func (a *api) telegramVideo(value *domainbot.Video) *tgmodels.Video {
	if value == nil {
		return nil
	}

	return &tgmodels.Video{
		FileID:       value.FileID,
		FileUniqueID: value.FileUniqueID,
		Width:        int(value.Width),
		Height:       int(value.Height),
		Duration:     value.Duration,
		MimeType:     value.MimeType,
		FileSize:     int64(value.FileSize),
	}
}

func (a *api) telegramAnimation(value *domainbot.Animation) *tgmodels.Animation {
	if value == nil {
		return nil
	}

	return &tgmodels.Animation{
		FileID:       value.FileID,
		FileUniqueID: value.FileUniqueID,
		Width:        int(value.Width),
		Height:       int(value.Height),
		Duration:     value.Duration,
		FileName:     value.FileName,
		MimeType:     value.MimeType,
		FileSize:     int64(value.FileSize),
	}
}

func (a *api) telegramAudio(value *domainbot.Audio) *tgmodels.Audio {
	if value == nil {
		return nil
	}

	return &tgmodels.Audio{
		FileID:       value.FileID,
		FileUniqueID: value.FileUniqueID,
		Duration:     value.Duration,
		FileName:     value.FileName,
		MimeType:     value.MimeType,
		FileSize:     int64(value.FileSize),
	}
}

func (a *api) telegramVideoNote(value *domainbot.VideoNote) *tgmodels.VideoNote {
	if value == nil {
		return nil
	}

	return &tgmodels.VideoNote{
		FileID:       value.FileID,
		FileUniqueID: value.FileUniqueID,
		Length:       int(value.Length),
		Duration:     value.Duration,
		FileSize:     int(value.FileSize),
	}
}

func (a *api) telegramVoice(value *domainbot.Voice) *tgmodels.Voice {
	if value == nil {
		return nil
	}

	return &tgmodels.Voice{
		FileID:       value.FileID,
		FileUniqueID: value.FileUniqueID,
		Duration:     value.Duration,
		MimeType:     value.MimeType,
		FileSize:     int64(value.FileSize),
	}
}

func (a *api) telegramSticker(value *domainbot.Sticker) *tgmodels.Sticker {
	if value == nil {
		return nil
	}

	return &tgmodels.Sticker{
		FileID:       value.FileID,
		FileUniqueID: value.FileUniqueID,
		Width:        int(value.Width),
		Height:       int(value.Height),
		Emoji:        value.Emoji,
		SetName:      value.SetName,
		FileSize:     int(value.FileSize),
	}
}

func (a *api) telegramLocation(value *domainbot.Location) *tgmodels.Location {
	if value == nil {
		return nil
	}

	return &tgmodels.Location{
		Longitude: value.Longitude,
		Latitude:  value.Latitude,
	}
}

func (a *api) telegramContact(ctx context.Context, value *domainbot.Contact) *tgmodels.Contact {
	if value == nil {
		return nil
	}

	result := &tgmodels.Contact{
		PhoneNumber: value.PhoneNumber,
		FirstName:   value.FirstName,
		LastName:    value.LastName,
		VCard:       value.VCard,
	}
	if value.UserID != "" {
		if publicID, err := a.bot.PublicAccountID(ctx, value.UserID); err == nil {
			result.UserID = publicID
		}
	}

	return result
}

func (a *api) telegramPoll(value *domainbot.Poll) *tgmodels.Poll {
	if value == nil {
		return nil
	}

	options := make([]tgmodels.PollOption, 0, len(value.Options))
	for _, option := range value.Options {
		options = append(options, tgmodels.PollOption{
			Text:       option.Text,
			VoterCount: option.VoterCount,
		})
	}

	return &tgmodels.Poll{
		ID:                    value.ID,
		Question:              value.Question,
		Options:               options,
		TotalVoterCount:       value.TotalVoterCount,
		IsClosed:              value.IsClosed,
		IsAnonymous:           value.IsAnonymous,
		Type:                  value.Type,
		AllowsMultipleAnswers: value.AllowsMultipleAnswers,
	}
}

func (a *api) telegramGame(value *domainbot.Game) *tgmodels.Game {
	if value == nil {
		return nil
	}

	photo := make([]tgmodels.PhotoSize, 0, len(value.Photo))
	for _, item := range value.Photo {
		photo = append(photo, tgmodels.PhotoSize{
			FileID:       item.FileID,
			FileUniqueID: item.FileUniqueID,
			Width:        int(item.Width),
			Height:       int(item.Height),
			FileSize:     int(item.FileSize),
		})
	}

	return &tgmodels.Game{
		Title:       value.Title,
		Description: value.Description,
		Photo:       photo,
		Text:        value.Text,
	}
}

func (a *api) telegramVenue(value *domainbot.Venue) *tgmodels.Venue {
	if value == nil {
		return nil
	}

	location := a.telegramLocation(&value.Location)
	if location == nil {
		return nil
	}

	return &tgmodels.Venue{
		Location:        *location,
		Title:           value.Title,
		Address:         value.Address,
		FoursquareID:    value.FoursquareID,
		FoursquareType:  value.FoursquareType,
		GooglePlaceID:   value.GooglePlaceID,
		GooglePlaceType: value.GooglePlaceType,
	}
}

func (a *api) telegramDice(value *domainbot.Dice) *tgmodels.Dice {
	if value == nil {
		return nil
	}

	return &tgmodels.Dice{
		Emoji: value.Emoji,
		Value: value.Value,
	}
}

func telegramReplyMarkup(value *domainbot.InlineKeyboardMarkup) *tgmodels.InlineKeyboardMarkup {
	if value == nil {
		return nil
	}

	result := &tgmodels.InlineKeyboardMarkup{
		InlineKeyboard: make([][]tgmodels.InlineKeyboardButton, 0, len(value.InlineKeyboard)),
	}
	for _, row := range value.InlineKeyboard {
		buttons := make([]tgmodels.InlineKeyboardButton, 0, len(row))
		for _, button := range row {
			buttons = append(buttons, tgmodels.InlineKeyboardButton{
				Text:         button.Text,
				URL:          button.URL,
				CallbackData: button.CallbackData,
			})
		}
		result.InlineKeyboard = append(result.InlineKeyboard, buttons)
	}

	return result
}

func telegramCommands(values []domainbot.Command) []tgmodels.BotCommand {
	result := make([]tgmodels.BotCommand, 0, len(values))
	for _, value := range values {
		result = append(result, tgmodels.BotCommand{
			Command:     value.Command,
			Description: value.Description,
		})
	}

	return result
}

func telegramMenu(value domainbot.MenuButton) *tgmodels.MenuButton {
	switch value.Type {
	case domainbot.MenuButtonCommands:
		return &tgmodels.MenuButton{
			Type:     tgmodels.MenuButtonTypeCommands,
			Commands: &tgmodels.MenuButtonCommands{},
		}
	case domainbot.MenuButtonWebApp:
		return &tgmodels.MenuButton{
			Type: tgmodels.MenuButtonTypeWebApp,
			WebApp: &tgmodels.MenuButtonWebApp{
				Text: value.Text,
				WebApp: tgmodels.WebAppInfo{
					URL: value.WebAppURL,
				},
			},
		}
	default:
		return &tgmodels.MenuButton{
			Type:    tgmodels.MenuButtonTypeDefault,
			Default: &tgmodels.MenuButtonDefault{},
		}
	}
}

func telegramFile(value domainbot.File) tgmodels.File {
	return tgmodels.File{
		FileID:       value.FileID,
		FileUniqueID: value.FileUniqueID,
		FileSize:     int64(value.FileSize),
		FilePath:     value.FilePath,
	}
}

func telegramAdminRights(value domainbot.AdminRights) *tgmodels.ChatAdministratorRights {
	return &tgmodels.ChatAdministratorRights{
		IsAnonymous:             value.IsAnonymous,
		CanManageChat:           value.CanManageChat,
		CanDeleteMessages:       value.CanDeleteMessages,
		CanManageVideoChats:     value.CanManageVideoChats,
		CanRestrictMembers:      value.CanRestrictMembers,
		CanPromoteMembers:       value.CanPromoteMembers,
		CanChangeInfo:           value.CanChangeInfo,
		CanInviteUsers:          value.CanInviteUsers,
		CanPostMessages:         value.CanPostMessages,
		CanEditMessages:         value.CanEditMessages,
		CanPinMessages:          value.CanPinMessages,
		CanPostStories:          value.CanPostStories,
		CanEditStories:          value.CanEditStories,
		CanDeleteStories:        value.CanDeleteStories,
		CanManageTopics:         value.CanManageTopics,
		CanManageDirectMessages: value.CanManageDirectMessages,
		CanManageTags:           value.CanManageTags,
	}
}

func botAdminRights(value tgmodels.ChatAdministratorRights) *domainbot.AdminRights {
	return &domainbot.AdminRights{
		IsAnonymous:             value.IsAnonymous,
		CanManageChat:           value.CanManageChat,
		CanDeleteMessages:       value.CanDeleteMessages,
		CanManageVideoChats:     value.CanManageVideoChats,
		CanRestrictMembers:      value.CanRestrictMembers,
		CanPromoteMembers:       value.CanPromoteMembers,
		CanChangeInfo:           value.CanChangeInfo,
		CanInviteUsers:          value.CanInviteUsers,
		CanPostMessages:         value.CanPostMessages,
		CanEditMessages:         value.CanEditMessages,
		CanPinMessages:          value.CanPinMessages,
		CanPostStories:          value.CanPostStories,
		CanEditStories:          value.CanEditStories,
		CanDeleteStories:        value.CanDeleteStories,
		CanManageTopics:         value.CanManageTopics,
		CanManageDirectMessages: value.CanManageDirectMessages,
		CanManageTags:           value.CanManageTags,
	}
}

func (a *api) internalChatID(ctx context.Context, value textID) (string, error) {
	return a.bot.ResolveChatID(ctx, string(value))
}

func (a *api) internalMessageID(ctx context.Context, value textID) (string, error) {
	return a.bot.ResolveMessageID(ctx, string(value))
}

func (a *api) internalTopicID(ctx context.Context, value textID) (string, error) {
	return a.bot.ResolveTopicID(ctx, string(value))
}

func (a *api) internalUserID(ctx context.Context, value textID) (string, error) {
	return a.bot.ResolveAccountID(ctx, string(value))
}

func publicMessageIDString(value int64) string {
	return strconv.FormatInt(value, 10)
}
