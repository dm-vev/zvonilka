package bot

import (
	"context"
	"strings"
	"time"
)

// CommandScopeType identifies one Telegram bot-command scope kind.
type CommandScopeType string

const (
	CommandScopeDefault               CommandScopeType = "default"
	CommandScopeAllPrivateChats       CommandScopeType = "all_private_chats"
	CommandScopeAllGroupChats         CommandScopeType = "all_group_chats"
	CommandScopeAllChatAdministrators CommandScopeType = "all_chat_administrators"
	CommandScopeChat                  CommandScopeType = "chat"
	CommandScopeChatAdministrators    CommandScopeType = "chat_administrators"
	CommandScopeChatMember            CommandScopeType = "chat_member"
)

// CommandScope describes one bot command scope projection.
type CommandScope struct {
	Type   CommandScopeType `json:"type"`
	ChatID string           `json:"chat_id,omitempty"`
	UserID string           `json:"user_id,omitempty"`
}

// Command describes one Telegram-shaped bot command.
type Command struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// CommandSet stores commands for one bot/scope/language tuple.
type CommandSet struct {
	BotAccountID string
	Scope        CommandScope
	LanguageCode string
	Commands     []Command
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SetCommandsParams describes one setMyCommands request.
type SetCommandsParams struct {
	BotToken     string
	Scope        CommandScope
	LanguageCode string
	Commands     []Command
}

// GetCommandsParams describes one getMyCommands request.
type GetCommandsParams struct {
	BotToken     string
	Scope        CommandScope
	LanguageCode string
}

// DeleteCommandsParams describes one deleteMyCommands request.
type DeleteCommandsParams struct {
	BotToken     string
	Scope        CommandScope
	LanguageCode string
}

// SetMyCommands stores bot commands for one scope.
func (s *Service) SetMyCommands(ctx context.Context, params SetCommandsParams) error {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return err
	}

	set, err := normalizeCommandSet(CommandSet{
		BotAccountID: account.ID,
		Scope:        params.Scope,
		LanguageCode: params.LanguageCode,
		Commands:     params.Commands,
		CreatedAt:    s.currentTime(),
		UpdatedAt:    s.currentTime(),
	}, s.currentTime())
	if err != nil {
		return err
	}

	_, err = s.store.SaveCommands(ctx, set)
	return err
}

// GetMyCommands resolves bot commands for one scope.
func (s *Service) GetMyCommands(ctx context.Context, params GetCommandsParams) ([]Command, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return nil, err
	}

	scope, languageCode, err := normalizeCommandLookup(params.Scope, params.LanguageCode)
	if err != nil {
		return nil, err
	}

	set, err := s.store.CommandsByScope(ctx, account.ID, scope, languageCode)
	if err != nil {
		if err == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	return append([]Command(nil), set.Commands...), nil
}

// DeleteMyCommands removes commands for one scope.
func (s *Service) DeleteMyCommands(ctx context.Context, params DeleteCommandsParams) error {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return err
	}

	scope, languageCode, err := normalizeCommandLookup(params.Scope, params.LanguageCode)
	if err != nil {
		return err
	}

	err = s.store.DeleteCommands(ctx, account.ID, scope, languageCode)
	if err == ErrNotFound {
		return nil
	}

	return err
}

func normalizeCommandLookup(scope CommandScope, languageCode string) (CommandScope, string, error) {
	scope, err := normalizeCommandScope(scope)
	if err != nil {
		return CommandScope{}, "", err
	}

	languageCode = strings.ToLower(strings.TrimSpace(languageCode))
	return scope, languageCode, nil
}

// NormalizeCommandLookup validates and normalizes one command lookup tuple.
func NormalizeCommandLookup(scope CommandScope, languageCode string) (CommandScope, string, error) {
	return normalizeCommandLookup(scope, languageCode)
}

func normalizeCommandSet(set CommandSet, now time.Time) (CommandSet, error) {
	set.BotAccountID = strings.TrimSpace(set.BotAccountID)
	if set.BotAccountID == "" {
		return CommandSet{}, ErrInvalidInput
	}

	scope, languageCode, err := normalizeCommandLookup(set.Scope, set.LanguageCode)
	if err != nil {
		return CommandSet{}, err
	}
	set.Scope = scope
	set.LanguageCode = languageCode
	if len(set.Commands) == 0 {
		return CommandSet{}, ErrInvalidInput
	}

	commands := make([]Command, 0, len(set.Commands))
	seen := make(map[string]struct{}, len(set.Commands))
	for _, command := range set.Commands {
		command.Command = strings.TrimSpace(strings.TrimPrefix(command.Command, "/"))
		command.Description = strings.TrimSpace(command.Description)
		if command.Command == "" || command.Description == "" {
			return CommandSet{}, ErrInvalidInput
		}
		if _, ok := seen[command.Command]; ok {
			return CommandSet{}, ErrConflict
		}
		seen[command.Command] = struct{}{}
		commands = append(commands, command)
	}
	set.Commands = commands

	if set.CreatedAt.IsZero() {
		set.CreatedAt = now.UTC()
	}
	if set.UpdatedAt.IsZero() {
		set.UpdatedAt = set.CreatedAt
	}

	return set, nil
}

// NormalizeCommandSet validates and normalizes one command set.
func NormalizeCommandSet(set CommandSet, now time.Time) (CommandSet, error) {
	return normalizeCommandSet(set, now)
}

func normalizeCommandScope(scope CommandScope) (CommandScope, error) {
	scope.Type = CommandScopeType(strings.ToLower(strings.TrimSpace(string(scope.Type))))
	scope.ChatID = strings.TrimSpace(scope.ChatID)
	scope.UserID = strings.TrimSpace(scope.UserID)
	if scope.Type == "" {
		scope.Type = CommandScopeDefault
	}

	switch scope.Type {
	case CommandScopeDefault,
		CommandScopeAllPrivateChats,
		CommandScopeAllGroupChats,
		CommandScopeAllChatAdministrators:
		if scope.ChatID != "" || scope.UserID != "" {
			return CommandScope{}, ErrInvalidInput
		}
	case CommandScopeChat, CommandScopeChatAdministrators:
		if scope.ChatID == "" || scope.UserID != "" {
			return CommandScope{}, ErrInvalidInput
		}
	case CommandScopeChatMember:
		if scope.ChatID == "" || scope.UserID == "" {
			return CommandScope{}, ErrInvalidInput
		}
	default:
		return CommandScope{}, ErrInvalidInput
	}

	return scope, nil
}
