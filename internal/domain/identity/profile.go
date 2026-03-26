package identity

import "context"

// UpdateProfile updates mutable account profile fields and reindexes the account document.
func (s *Service) UpdateProfile(ctx context.Context, params UpdateProfileParams) (Account, error) {
	if err := s.validateContext(ctx, "update profile"); err != nil {
		return Account{}, err
	}
	if params.AccountID == "" {
		return Account{}, ErrInvalidInput
	}

	var saved Account
	err := s.store.WithinTx(ctx, func(tx Store) error {
		account, err := s.lockAccount(ctx, tx, params.AccountID)
		if err != nil {
			return err
		}
		if account.Status != AccountStatusActive {
			return ErrForbidden
		}

		now := params.RequestedAt
		if now.IsZero() {
			now = s.currentTime()
		}

		username, email, phone := s.normalizeAccountInput(params.Username, params.Email, params.Phone)
		if username == "" {
			username = account.Username
		}
		if email == "" {
			email = account.Email
		}
		if phone == "" {
			phone = account.Phone
		}

		account.Username = username
		account.DisplayName = trimmedOrDefault(params.DisplayName, account.DisplayName)
		account.Bio = trimmed(params.Bio)
		account.Email = email
		account.Phone = phone
		account.CustomBadgeEmoji = trimmed(params.CustomBadgeEmoji)
		account.UpdatedAt = now

		saved, err = tx.SaveAccount(ctx, account)
		return err
	})
	if err != nil {
		return Account{}, err
	}

	s.indexAccount(ctx, saved)
	return saved, nil
}

func trimmedOrDefault(value string, fallback string) string {
	value = trimmed(value)
	if value == "" {
		return fallback
	}
	return value
}
