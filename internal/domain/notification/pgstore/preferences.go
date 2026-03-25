package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

// SavePreference inserts or updates account-level notification preferences.
func (s *Store) SavePreference(ctx context.Context, preference notification.Preference) (notification.Preference, error) {
	if err := s.requireStore(); err != nil {
		return notification.Preference{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.Preference{}, err
	}
	if s.tx != nil {
		return s.savePreference(ctx, preference)
	}

	var saved notification.Preference
	err := s.WithinTx(ctx, func(tx notification.Store) error {
		var saveErr error
		saved, saveErr = tx.(*Store).savePreference(ctx, preference)
		return saveErr
	})
	if err != nil {
		return notification.Preference{}, err
	}

	return saved, nil
}

func (s *Store) savePreference(ctx context.Context, preference notification.Preference) (notification.Preference, error) {
	now := time.Now().UTC()
	preference, err := notification.NormalizePreference(preference, now)
	if err != nil {
		return notification.Preference{}, err
	}

	query := fmt.Sprintf(`
INSERT INTO %s (
	account_id,
	enabled,
	direct_enabled,
	group_enabled,
	channel_enabled,
	mention_enabled,
	reply_enabled,
	quiet_hours_enabled,
	quiet_hours_start_minute,
	quiet_hours_end_minute,
	quiet_hours_timezone,
	muted_until,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
ON CONFLICT (account_id) DO UPDATE SET
	enabled = EXCLUDED.enabled,
	direct_enabled = EXCLUDED.direct_enabled,
	group_enabled = EXCLUDED.group_enabled,
	channel_enabled = EXCLUDED.channel_enabled,
	mention_enabled = EXCLUDED.mention_enabled,
	reply_enabled = EXCLUDED.reply_enabled,
	quiet_hours_enabled = EXCLUDED.quiet_hours_enabled,
	quiet_hours_start_minute = EXCLUDED.quiet_hours_start_minute,
	quiet_hours_end_minute = EXCLUDED.quiet_hours_end_minute,
	quiet_hours_timezone = EXCLUDED.quiet_hours_timezone,
	muted_until = EXCLUDED.muted_until,
	updated_at = EXCLUDED.updated_at
RETURNING account_id, enabled, direct_enabled, group_enabled, channel_enabled, mention_enabled, reply_enabled,
	quiet_hours_enabled, quiet_hours_start_minute, quiet_hours_end_minute, quiet_hours_timezone, muted_until, updated_at
`, s.table("notification_preferences"))

	row := s.conn().QueryRowContext(
		ctx,
		query,
		preference.AccountID,
		preference.Enabled,
		preference.DirectEnabled,
		preference.GroupEnabled,
		preference.ChannelEnabled,
		preference.MentionEnabled,
		preference.ReplyEnabled,
		preference.QuietHours.Enabled,
		preference.QuietHours.StartMinute,
		preference.QuietHours.EndMinute,
		preference.QuietHours.Timezone,
		encodeTime(preference.MutedUntil),
		preference.UpdatedAt.UTC(),
	)

	saved, err := scanPreference(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.Preference{}, notification.ErrNotFound
		}
		if mappedErr := mapConstraintError(err); mappedErr != nil {
			return notification.Preference{}, mappedErr
		}

		return notification.Preference{}, fmt.Errorf("save notification preference for account %s: %w", preference.AccountID, err)
	}

	return saved, nil
}

// PreferenceByAccountID resolves one preference row by account ID.
func (s *Store) PreferenceByAccountID(ctx context.Context, accountID string) (notification.Preference, error) {
	if err := s.requireStore(); err != nil {
		return notification.Preference{}, err
	}
	if err := s.requireContext(ctx); err != nil {
		return notification.Preference{}, err
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return notification.Preference{}, notification.ErrInvalidInput
	}

	query := fmt.Sprintf(`
SELECT account_id, enabled, direct_enabled, group_enabled, channel_enabled, mention_enabled, reply_enabled,
	quiet_hours_enabled, quiet_hours_start_minute, quiet_hours_end_minute, quiet_hours_timezone, muted_until, updated_at
FROM %s
WHERE account_id = $1
`, s.table("notification_preferences"))

	row := s.conn().QueryRowContext(ctx, query, accountID)
	preference, err := scanPreference(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return notification.Preference{}, notification.ErrNotFound
		}
		return notification.Preference{}, fmt.Errorf("load notification preference for account %s: %w", accountID, err)
	}

	return preference, nil
}
