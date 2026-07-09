package pgstore

import (
	"context"
	"fmt"
	"strings"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

func (s *Store) SavePrivacy(ctx context.Context, privacy domainuser.Privacy) (domainuser.Privacy, error) {
	if err := s.requireStore(); err != nil {
		return domainuser.Privacy{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return domainuser.Privacy{}, err
	}
	if strings.TrimSpace(privacy.AccountID) == "" {
		return domainuser.Privacy{}, domainuser.ErrInvalidInput
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	account_id, phone_visibility, last_seen_visibility, message_privacy, birthday_visibility,
	allow_contact_sync, allow_unknown_senders, allow_username_search, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (account_id) DO UPDATE SET
	phone_visibility = EXCLUDED.phone_visibility,
	last_seen_visibility = EXCLUDED.last_seen_visibility,
	message_privacy = EXCLUDED.message_privacy,
	birthday_visibility = EXCLUDED.birthday_visibility,
	allow_contact_sync = EXCLUDED.allow_contact_sync,
	allow_unknown_senders = EXCLUDED.allow_unknown_senders,
	allow_username_search = EXCLUDED.allow_username_search,
	updated_at = EXCLUDED.updated_at
RETURNING %s
`, s.table("user_privacy"), privacyColumnList)

	saved, err := scanPrivacy(s.conn().QueryRowContext(ctx, query,
		privacy.AccountID,
		privacy.PhoneVisibility,
		privacy.LastSeenVisibility,
		privacy.MessagePrivacy,
		privacy.BirthdayVisibility,
		privacy.AllowContactSync,
		privacy.AllowUnknownSenders,
		privacy.AllowUsernameSearch,
		privacy.CreatedAt.UTC(),
		privacy.UpdatedAt.UTC(),
	))
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return domainuser.Privacy{}, mapped
		}
		return domainuser.Privacy{}, fmt.Errorf("save privacy %s: %w", privacy.AccountID, err)
	}
	return saved, nil
}

func (s *Store) PrivacyByAccountID(ctx context.Context, accountID string) (domainuser.Privacy, error) {
	if err := s.requireStore(); err != nil {
		return domainuser.Privacy{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return domainuser.Privacy{}, err
	}
	if strings.TrimSpace(accountID) == "" {
		return domainuser.Privacy{}, domainuser.ErrNotFound
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE account_id = $1`, privacyColumnList, s.table("user_privacy"))
	privacy, err := scanPrivacy(s.conn().QueryRowContext(ctx, query, accountID))
	if err != nil {
		if isNoRows(err) {
			return domainuser.Privacy{}, domainuser.ErrNotFound
		}
		return domainuser.Privacy{}, fmt.Errorf("load privacy %s: %w", accountID, err)
	}
	return privacy, nil
}
