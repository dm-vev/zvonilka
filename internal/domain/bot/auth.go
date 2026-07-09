package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
)

// GetMe resolves the authenticated bot profile.
func (s *Service) GetMe(ctx context.Context, token string) (User, error) {
	account, err := s.botAccount(ctx, token)
	if err != nil {
		return User{}, err
	}

	return userFromAccount(account), nil
}

func (s *Service) botAccount(ctx context.Context, token string) (identity.Account, error) {
	if err := s.validateContext(ctx, "resolve bot account"); err != nil {
		return identity.Account{}, err
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return identity.Account{}, ErrInvalidInput
	}

	account, err := s.identity.BotAccountByToken(ctx, token)
	if err != nil {
		return identity.Account{}, fmt.Errorf("resolve bot account: %w", mapIdentityError(err))
	}

	return account, nil
}
