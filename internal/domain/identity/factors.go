package identity

import (
	"context"
	"fmt"
	"strings"
)

// GetLoginOptions resolves the supported login factors for one identifier.
func (s *Service) GetLoginOptions(ctx context.Context, params GetLoginOptionsParams) (LoginOptionsResult, error) {
	if err := s.validateContext(ctx, "get login options"); err != nil {
		return LoginOptionsResult{}, err
	}

	username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
	account, err := s.lookupAccountByIdentifier(ctx, username, email, phone)
	if err != nil {
		return LoginOptionsResult{}, err
	}
	if account.Status != AccountStatusActive {
		return LoginOptionsResult{}, ErrForbidden
	}

	result := LoginOptionsResult{
		AccountKind: account.Kind,
	}
	if account.Kind == AccountKindBot {
		result.Options = []LoginOption{{
			Factor:   LoginFactorBotToken,
			Required: true,
		}}
		return result, nil
	}

	channels := s.loginChannels(ctx, account)
	if len(channels) > 0 {
		result.Options = append(result.Options, LoginOption{
			Factor:   LoginFactorCode,
			Required: true,
			Channels: channels,
		})
	}

	if _, err := s.store.AccountCredentialByAccountID(ctx, account.ID, AccountCredentialKindRecovery); err == nil {
		result.PasswordRecoveryEnabled = true
		result.Options = append(result.Options, LoginOption{
			Factor:   LoginFactorRecoveryPassword,
			Required: false,
		})
	} else if err != ErrNotFound {
		return LoginOptionsResult{}, fmt.Errorf("load recovery credential for %s: %w", account.ID, err)
	}

	if _, err := s.store.AccountCredentialByAccountID(ctx, account.ID, AccountCredentialKindPassword); err == nil {
		result.Options = append(result.Options, LoginOption{
			Factor:   LoginFactorPassword,
			Required: false,
		})
	} else if err != ErrNotFound {
		return LoginOptionsResult{}, fmt.Errorf("load password credential for %s: %w", account.ID, err)
	}

	if len(result.Options) == 0 {
		return LoginOptionsResult{}, ErrInvalidInput
	}

	return result, nil
}

func (s *Service) loginChannels(ctx context.Context, account Account) []LoginDeliveryChannel {
	channels := make([]LoginDeliveryChannel, 0, 3)
	if account.Email != "" {
		channels = append(channels, LoginDeliveryChannelEmail)
	}
	if account.Phone != "" {
		channels = append(channels, LoginDeliveryChannelSMS)
	}

	devices, err := s.store.DevicesByAccountID(ctx, account.ID)
	if err != nil {
		return channels
	}
	for _, device := range devices {
		if strings.TrimSpace(device.PushToken) == "" {
			continue
		}
		if device.Status != DeviceStatusActive {
			continue
		}
		channels = append(channels, LoginDeliveryChannelPush)
		break
	}

	return channels
}
