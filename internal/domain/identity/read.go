package identity

import (
	"context"
	"fmt"
	"strings"
)

// AccountByID resolves one account by primary key.
func (s *Service) AccountByID(ctx context.Context, accountID string) (Account, error) {
	if err := s.validateContext(ctx, "get account"); err != nil {
		return Account{}, err
	}

	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return Account{}, ErrInvalidInput
	}

	account, err := s.store.AccountByID(ctx, accountID)
	if err != nil {
		return Account{}, fmt.Errorf("load account %s: %w", accountID, err)
	}

	return account, nil
}

// AccountByUsername resolves one account by normalized username.
func (s *Service) AccountByUsername(ctx context.Context, username string) (Account, error) {
	if err := s.validateContext(ctx, "get account by username"); err != nil {
		return Account{}, err
	}

	username = normalizeUsername(username)
	if username == "" {
		return Account{}, ErrInvalidInput
	}

	account, err := s.store.AccountByUsername(ctx, username)
	if err != nil {
		return Account{}, fmt.Errorf("load account by username %s: %w", username, err)
	}

	return account, nil
}
