package identity

import (
	"context"
	"fmt"
	"strings"
)

// BotAccountByToken resolves one active bot account by its issued bot token.
func (s *Service) BotAccountByToken(ctx context.Context, token string) (Account, error) {
	if err := s.validateContext(ctx, "get bot account by token"); err != nil {
		return Account{}, err
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return Account{}, ErrInvalidInput
	}

	account, err := s.store.AccountByBotTokenHash(ctx, hashSecret(token))
	if err != nil {
		return Account{}, fmt.Errorf("load bot account by token: %w", err)
	}
	if account.Kind != AccountKindBot {
		return Account{}, ErrForbidden
	}
	if account.Status != AccountStatusActive {
		return Account{}, ErrForbidden
	}

	return account, nil
}
