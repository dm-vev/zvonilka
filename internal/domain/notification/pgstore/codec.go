package pgstore

import (
	"database/sql"
	"strings"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func encodeTime(value time.Time) sql.NullTime {
	if value.IsZero() {
		return sql.NullTime{}
	}

	return sql.NullTime{Time: value.UTC(), Valid: true}
}

func decodeTime(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}

	return value.Time.UTC()
}

func scanPreference(row rowScanner) (notification.Preference, error) {
	var (
		preference         notification.Preference
		quietHoursEnabled  bool
		quietHoursStart    int
		quietHoursEnd      int
		quietHoursTimezone string
		mutedUntil         sql.NullTime
	)

	if err := row.Scan(
		&preference.AccountID,
		&preference.Enabled,
		&preference.DirectEnabled,
		&preference.GroupEnabled,
		&preference.ChannelEnabled,
		&preference.MentionEnabled,
		&preference.ReplyEnabled,
		&quietHoursEnabled,
		&quietHoursStart,
		&quietHoursEnd,
		&quietHoursTimezone,
		&mutedUntil,
		&preference.UpdatedAt,
	); err != nil {
		return notification.Preference{}, err
	}

	if quietHoursEnabled {
		preference.QuietHours = notification.QuietHours{
			Enabled:     true,
			StartMinute: quietHoursStart,
			EndMinute:   quietHoursEnd,
			Timezone:    strings.TrimSpace(quietHoursTimezone),
		}
	}
	preference.MutedUntil = decodeTime(mutedUntil)
	preference.UpdatedAt = preference.UpdatedAt.UTC()

	return preference, nil
}

func scanOverride(row rowScanner) (notification.ConversationOverride, error) {
	var (
		override   notification.ConversationOverride
		mutedUntil sql.NullTime
	)

	if err := row.Scan(
		&override.ConversationID,
		&override.AccountID,
		&override.Muted,
		&override.MentionsOnly,
		&mutedUntil,
		&override.UpdatedAt,
	); err != nil {
		return notification.ConversationOverride{}, err
	}

	override.MutedUntil = decodeTime(mutedUntil)
	override.UpdatedAt = override.UpdatedAt.UTC()

	return override, nil
}

func scanPushToken(row rowScanner) (notification.PushToken, error) {
	var (
		token     notification.PushToken
		revokedAt sql.NullTime
	)

	if err := row.Scan(
		&token.ID,
		&token.AccountID,
		&token.DeviceID,
		&token.Provider,
		&token.Token,
		&token.Platform,
		&token.Enabled,
		&token.CreatedAt,
		&token.UpdatedAt,
		&revokedAt,
	); err != nil {
		return notification.PushToken{}, err
	}

	token.CreatedAt = token.CreatedAt.UTC()
	token.UpdatedAt = token.UpdatedAt.UTC()
	token.RevokedAt = decodeTime(revokedAt)

	return token, nil
}

func scanDelivery(row rowScanner) (notification.Delivery, error) {
	var (
		delivery       notification.Delivery
		leaseExpiresAt sql.NullTime
		lastAttemptAt  sql.NullTime
	)

	if err := row.Scan(
		&delivery.ID,
		&delivery.DedupKey,
		&delivery.EventID,
		&delivery.ConversationID,
		&delivery.MessageID,
		&delivery.AccountID,
		&delivery.DeviceID,
		&delivery.PushTokenID,
		&delivery.Kind,
		&delivery.Reason,
		&delivery.Mode,
		&delivery.State,
		&delivery.Priority,
		&delivery.Attempts,
		&delivery.NextAttemptAt,
		&delivery.LeaseToken,
		&leaseExpiresAt,
		&lastAttemptAt,
		&delivery.LastError,
		&delivery.CreatedAt,
		&delivery.UpdatedAt,
	); err != nil {
		return notification.Delivery{}, err
	}

	delivery.NextAttemptAt = delivery.NextAttemptAt.UTC()
	delivery.LeaseExpiresAt = decodeTime(leaseExpiresAt)
	delivery.LastAttemptAt = decodeTime(lastAttemptAt)
	delivery.CreatedAt = delivery.CreatedAt.UTC()
	delivery.UpdatedAt = delivery.UpdatedAt.UTC()

	return delivery, nil
}

func scanCursor(row rowScanner) (notification.WorkerCursor, error) {
	var cursor notification.WorkerCursor

	if err := row.Scan(&cursor.Name, &cursor.LastSequence, &cursor.UpdatedAt); err != nil {
		return notification.WorkerCursor{}, err
	}

	cursor.UpdatedAt = cursor.UpdatedAt.UTC()

	return cursor, nil
}
