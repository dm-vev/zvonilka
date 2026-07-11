package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
)

const accountSettingsColumnList = `account_id, account_ttl_days, inactive_session_ttl_days,
	default_reaction, default_message_auto_delete_seconds, auto_download, autosave, browser,
	reaction_notifications, allow_new_chats_from_unknown_users, incoming_paid_message_star_count,
	created_at, updated_at`

func (s *Store) SaveAccountSettings(ctx context.Context, settings domainuser.AccountSettings) (domainuser.AccountSettings, error) {
	if err := s.requireStore(); err != nil {
		return domainuser.AccountSettings{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return domainuser.AccountSettings{}, err
	}
	if strings.TrimSpace(settings.AccountID) == "" {
		return domainuser.AccountSettings{}, domainuser.ErrInvalidInput
	}
	autoDownload, err := json.Marshal(settings.AutoDownload)
	if err != nil {
		return domainuser.AccountSettings{}, fmt.Errorf("encode auto-download settings: %w", err)
	}
	autosave, err := json.Marshal(settings.Autosave)
	if err != nil {
		return domainuser.AccountSettings{}, fmt.Errorf("encode autosave settings: %w", err)
	}
	browser, err := json.Marshal(settings.Browser)
	if err != nil {
		return domainuser.AccountSettings{}, fmt.Errorf("encode browser settings: %w", err)
	}
	reactions, err := json.Marshal(settings.ReactionNotifications)
	if err != nil {
		return domainuser.AccountSettings{}, fmt.Errorf("encode reaction notification settings: %w", err)
	}

	query := fmt.Sprintf(`
INSERT INTO %s (%s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (account_id) DO UPDATE SET
	account_ttl_days = EXCLUDED.account_ttl_days,
	inactive_session_ttl_days = EXCLUDED.inactive_session_ttl_days,
	default_reaction = EXCLUDED.default_reaction,
	default_message_auto_delete_seconds = EXCLUDED.default_message_auto_delete_seconds,
	auto_download = EXCLUDED.auto_download,
	autosave = EXCLUDED.autosave,
	browser = EXCLUDED.browser,
	reaction_notifications = EXCLUDED.reaction_notifications,
	allow_new_chats_from_unknown_users = EXCLUDED.allow_new_chats_from_unknown_users,
	incoming_paid_message_star_count = EXCLUDED.incoming_paid_message_star_count,
	updated_at = EXCLUDED.updated_at
RETURNING %s`, s.table("user_account_settings"), accountSettingsColumnList, accountSettingsColumnList)

	result, err := scanAccountSettings(s.conn().QueryRowContext(ctx, query,
		settings.AccountID, settings.AccountTTLDays, settings.InactiveSessionTTLDays,
		settings.DefaultReaction, settings.DefaultMessageAutoDeleteSeconds,
		autoDownload, autosave, browser, reactions,
		settings.AllowNewChatsFromUnknownUsers, settings.IncomingPaidMessageStarCount,
		settings.CreatedAt.UTC(), settings.UpdatedAt.UTC(),
	))
	if err != nil {
		if mapped := mapConstraintError(err); mapped != nil {
			return domainuser.AccountSettings{}, mapped
		}
		return domainuser.AccountSettings{}, fmt.Errorf("save account settings %s: %w", settings.AccountID, err)
	}
	return result, nil
}

func (s *Store) AccountSettingsByAccountID(ctx context.Context, accountID string) (domainuser.AccountSettings, error) {
	if err := s.requireStore(); err != nil {
		return domainuser.AccountSettings{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return domainuser.AccountSettings{}, err
	}
	if strings.TrimSpace(accountID) == "" {
		return domainuser.AccountSettings{}, domainuser.ErrNotFound
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE account_id = $1`, accountSettingsColumnList, s.table("user_account_settings"))
	result, err := scanAccountSettings(s.conn().QueryRowContext(ctx, query, accountID))
	if err != nil {
		if isNoRows(err) {
			return domainuser.AccountSettings{}, domainuser.ErrNotFound
		}
		return domainuser.AccountSettings{}, fmt.Errorf("load account settings %s: %w", accountID, err)
	}
	return result, nil
}

type accountSettingsScanner interface {
	Scan(...any) error
}

func scanAccountSettings(row accountSettingsScanner) (domainuser.AccountSettings, error) {
	var settings domainuser.AccountSettings
	var autoDownload, autosave, browser, reactions []byte
	err := row.Scan(
		&settings.AccountID, &settings.AccountTTLDays, &settings.InactiveSessionTTLDays,
		&settings.DefaultReaction, &settings.DefaultMessageAutoDeleteSeconds,
		&autoDownload, &autosave, &browser, &reactions,
		&settings.AllowNewChatsFromUnknownUsers, &settings.IncomingPaidMessageStarCount,
		&settings.CreatedAt, &settings.UpdatedAt,
	)
	if err != nil {
		return domainuser.AccountSettings{}, err
	}
	if err := json.Unmarshal(autoDownload, &settings.AutoDownload); err != nil {
		return domainuser.AccountSettings{}, fmt.Errorf("decode auto-download settings: %w", err)
	}
	if err := json.Unmarshal(autosave, &settings.Autosave); err != nil {
		return domainuser.AccountSettings{}, fmt.Errorf("decode autosave settings: %w", err)
	}
	if err := json.Unmarshal(browser, &settings.Browser); err != nil {
		return domainuser.AccountSettings{}, fmt.Errorf("decode browser settings: %w", err)
	}
	if err := json.Unmarshal(reactions, &settings.ReactionNotifications); err != nil {
		return domainuser.AccountSettings{}, fmt.Errorf("decode reaction notification settings: %w", err)
	}
	return settings, nil
}
