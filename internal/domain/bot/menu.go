package bot

import (
	"context"
	"strings"
	"time"
)

// MenuButtonType identifies one supported menu button kind.
type MenuButtonType string

const (
	MenuButtonDefault  MenuButtonType = "default"
	MenuButtonCommands MenuButtonType = "commands"
	MenuButtonWebApp   MenuButtonType = "web_app"
)

// MenuButton describes one Telegram-shaped menu button.
type MenuButton struct {
	Type      MenuButtonType `json:"type"`
	Text      string         `json:"text,omitempty"`
	WebAppURL string         `json:"web_app_url,omitempty"`
}

// MenuState stores one chat-scoped menu button override.
type MenuState struct {
	BotAccountID string
	ChatID       string
	Button       MenuButton
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SetMenuParams describes one setChatMenuButton request.
type SetMenuParams struct {
	BotToken string
	ChatID   string
	Button   MenuButton
}

// GetMenuParams describes one getChatMenuButton request.
type GetMenuParams struct {
	BotToken string
	ChatID   string
}

// SetChatMenuButton stores one default or chat-specific menu button.
func (s *Service) SetChatMenuButton(ctx context.Context, params SetMenuParams) error {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return err
	}

	state, err := normalizeMenuState(MenuState{
		BotAccountID: account.ID,
		ChatID:       params.ChatID,
		Button:       params.Button,
		CreatedAt:    s.currentTime(),
		UpdatedAt:    s.currentTime(),
	}, s.currentTime())
	if err != nil {
		return err
	}

	_, err = s.store.SaveMenu(ctx, state)
	return err
}

// GetChatMenuButton resolves one default or chat-specific menu button.
func (s *Service) GetChatMenuButton(ctx context.Context, params GetMenuParams) (MenuButton, error) {
	account, err := s.botAccount(ctx, params.BotToken)
	if err != nil {
		return MenuButton{}, err
	}

	chatID := strings.TrimSpace(params.ChatID)
	if chatID != "" {
		state, err := s.store.MenuByChat(ctx, account.ID, chatID)
		if err == nil {
			return state.Button, nil
		}
		if err != ErrNotFound {
			return MenuButton{}, err
		}
	}

	state, err := s.store.MenuByChat(ctx, account.ID, "")
	if err == nil {
		return state.Button, nil
	}
	if err != ErrNotFound {
		return MenuButton{}, err
	}

	return MenuButton{Type: MenuButtonDefault}, nil
}

func normalizeMenuState(state MenuState, now time.Time) (MenuState, error) {
	state.BotAccountID = strings.TrimSpace(state.BotAccountID)
	state.ChatID = strings.TrimSpace(state.ChatID)
	button, err := normalizeMenuButton(state.Button)
	if err != nil {
		return MenuState{}, err
	}
	if state.BotAccountID == "" {
		return MenuState{}, ErrInvalidInput
	}
	state.Button = button
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now.UTC()
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = state.CreatedAt
	}

	return state, nil
}

// NormalizeMenuState validates and normalizes one menu-button row.
func NormalizeMenuState(state MenuState, now time.Time) (MenuState, error) {
	return normalizeMenuState(state, now)
}

func normalizeMenuButton(button MenuButton) (MenuButton, error) {
	button.Type = MenuButtonType(strings.ToLower(strings.TrimSpace(string(button.Type))))
	button.Text = strings.TrimSpace(button.Text)
	button.WebAppURL = strings.TrimSpace(button.WebAppURL)
	if button.Type == "" {
		button.Type = MenuButtonDefault
	}

	switch button.Type {
	case MenuButtonDefault, MenuButtonCommands:
		if button.Text != "" || button.WebAppURL != "" {
			return MenuButton{}, ErrInvalidInput
		}
	case MenuButtonWebApp:
		if button.Text == "" || button.WebAppURL == "" {
			return MenuButton{}, ErrInvalidInput
		}
	default:
		return MenuButton{}, ErrInvalidInput
	}

	return button, nil
}
