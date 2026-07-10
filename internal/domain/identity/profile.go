package identity

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const usernameChangeCooldown = 24 * time.Hour

// UpdateProfileResult contains the saved account and whether the request was replayed.
type UpdateProfileResult struct {
	Account  Account
	Replayed bool
}

// UpdateProfile updates mutable account profile fields and reindexes the account document.
func (s *Service) UpdateProfile(ctx context.Context, params UpdateProfileParams) (Account, error) {
	result, err := s.UpdateProfileWithResult(ctx, params)
	if err != nil {
		return Account{}, err
	}

	return result.Account, nil
}

// UpdateProfileWithResult updates a profile while exposing idempotency replay state.
func (s *Service) UpdateProfileWithResult(
	ctx context.Context,
	params UpdateProfileParams,
) (UpdateProfileResult, error) {
	if err := s.validateContext(ctx, "update profile"); err != nil {
		return UpdateProfileResult{}, err
	}
	if params.AccountID == "" {
		return UpdateProfileResult{}, ErrInvalidInput
	}

	fingerprint := updateProfileFingerprint(params)
	if params.IdempotencyKey != "" {
		cached, ok, err := s.idempotency.updateProfileResult(
			params.IdempotencyKey,
			fingerprint,
			s.currentTime(),
		)
		if err != nil {
			return UpdateProfileResult{}, err
		}
		if ok {
			if _, err := s.store.AccountByID(ctx, cached.account.ID); err != nil {
				return UpdateProfileResult{}, fmt.Errorf("verify cached profile %s: %w", cached.account.ID, err)
			}
			return UpdateProfileResult{Account: cached.account, Replayed: true}, nil
		}
	}

	if err := validateProfileUsername(params); err != nil {
		return UpdateProfileResult{}, err
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
		usernameRequested := params.UsernameSet || username != ""
		if usernameRequested && account.Username != username {
			if !account.UsernameChangedAt.IsZero() && now.Before(account.UsernameChangedAt.Add(usernameChangeCooldown)) {
				return ErrConflict
			}
			account.Username = username
			account.UsernameChangedAt = now
		}
		if email != "" {
			account.Email = email
		}
		if phone != "" {
			account.Phone = phone
		}

		if params.DisplayNameSet {
			account.DisplayName = trimmed(params.DisplayName)
		} else {
			account.DisplayName = trimmedOrDefault(params.DisplayName, account.DisplayName)
		}
		if params.BioSet {
			account.Bio = trimmed(params.Bio)
		} else if len(params.FieldMask) == 0 {
			account.Bio = trimmed(params.Bio)
		}
		if params.CustomBadgeSet || params.CustomBadgeEmoji != "" || len(params.FieldMask) == 0 {
			account.CustomBadgeEmoji = trimmed(params.CustomBadgeEmoji)
		}
		if params.AvatarMediaID != nil {
			account.AvatarMediaID = strings.TrimSpace(*params.AvatarMediaID)
		}
		account.UpdatedAt = now

		saved, err = tx.SaveAccount(ctx, account)
		return err
	})
	if err != nil {
		return UpdateProfileResult{}, err
	}

	s.indexAccount(ctx, saved)
	result := UpdateProfileResult{Account: saved}
	if params.IdempotencyKey != "" {
		s.idempotency.storeUpdateProfileResult(
			params.IdempotencyKey,
			fingerprint,
			updateProfileCacheResult{account: saved},
			s.currentTime(),
		)
	}

	return result, nil
}

func validateProfileUsername(params UpdateProfileParams) error {
	if !params.UsernameSet && params.Username == "" {
		return nil
	}

	return validateUsername(normalizeUsername(params.Username))
}

func trimmedOrDefault(value string, fallback string) string {
	value = trimmed(value)
	if value == "" {
		return fallback
	}
	return value
}
