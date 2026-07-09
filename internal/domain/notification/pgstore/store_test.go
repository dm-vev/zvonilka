package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	store, err := New(db, "notif")
	require.NoError(t, err)

	return store, mock, db
}

func TestMapConstraintError(t *testing.T) {
	t.Parallel()

	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23505"}), notification.ErrConflict))
	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23503"}), notification.ErrNotFound))
	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23514"}), notification.ErrInvalidInput))
	require.Nil(t, mapConstraintError(&pgconn.PgError{Code: "99999"}))
}

func TestSavePreferenceRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "notif"\."notification_preferences".*RETURNING account_id, enabled, direct_enabled, group_enabled, channel_enabled, mention_enabled, reply_enabled, quiet_hours_enabled, quiet_hours_start_minute, quiet_hours_end_minute, quiet_hours_timezone, muted_until, updated_at`).
		WithArgs(
			sqlmock.AnyArg(),
			true,
			false,
			true,
			true,
			true,
			false,
			true,
			22*60,
			7*60,
			"UTC",
			sqlmock.AnyArg(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id",
			"enabled",
			"direct_enabled",
			"group_enabled",
			"channel_enabled",
			"mention_enabled",
			"reply_enabled",
			"quiet_hours_enabled",
			"quiet_hours_start_minute",
			"quiet_hours_end_minute",
			"quiet_hours_timezone",
			"muted_until",
			"updated_at",
		}).AddRow(
			"acc-1",
			true,
			false,
			true,
			true,
			true,
			false,
			true,
			22*60,
			7*60,
			"UTC",
			nil,
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SavePreference(context.Background(), notification.Preference{
		AccountID:      "acc-1",
		Enabled:        true,
		DirectEnabled:  false,
		GroupEnabled:   true,
		ChannelEnabled: true,
		MentionEnabled: true,
		ReplyEnabled:   false,
		QuietHours: notification.QuietHours{
			Enabled:     true,
			StartMinute: 22 * 60,
			EndMinute:   7 * 60,
		},
		MutedUntil: now.Add(time.Hour),
		UpdatedAt:  now,
	})
	require.NoError(t, err)
	require.Equal(t, "acc-1", saved.AccountID)
	require.True(t, saved.QuietHours.Enabled)
	require.Equal(t, "UTC", saved.QuietHours.Timezone)
	require.True(t, saved.UpdatedAt.Equal(now))

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDeliveryRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "notif"\."notification_deliveries".*ON CONFLICT \(dedup_key\) DO UPDATE SET.*RETURNING.*lease_token.*lease_expires_at.*updated_at`).
		WithArgs(
			"del-1",
			"evt-1:conv-1:msg-1:acc-1::group:group",
			"evt-1",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			notification.NotificationKindGroup,
			"group",
			notification.DeliveryModeInApp,
			notification.DeliveryStateQueued,
			10,
			0,
			now.UTC(),
			"",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"",
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"dedup_key",
			"event_id",
			"conversation_id",
			"message_id",
			"account_id",
			"device_id",
			"push_token_id",
			"kind",
			"reason",
			"mode",
			"state",
			"priority",
			"attempts",
			"next_attempt_at",
			"lease_token",
			"lease_expires_at",
			"last_attempt_at",
			"last_error",
			"created_at",
			"updated_at",
		}).AddRow(
			"del-1",
			"evt-1:conv-1:msg-1:acc-1::group:group",
			"evt-1",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			notification.NotificationKindGroup,
			"group",
			notification.DeliveryModeInApp,
			notification.DeliveryStateQueued,
			10,
			0,
			now.UTC(),
			"",
			nil,
			nil,
			"",
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveDelivery(context.Background(), notification.Delivery{
		ID:             "del-1",
		DedupKey:       "evt-1:conv-1:msg-1:acc-1::group:group",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindGroup,
		Reason:         "group",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       10,
		NextAttemptAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	require.NoError(t, err)
	require.Equal(t, "del-1", saved.ID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeliveriesDueOrdering(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT.*lease_token.*lease_expires_at.*FROM "notif"\."notification_deliveries" WHERE state = \$1 AND next_attempt_at <= \$2 AND \(lease_expires_at IS NULL OR lease_expires_at <= \$2\) ORDER BY priority DESC, next_attempt_at ASC, created_at ASC, id ASC LIMIT \$3`).
		WithArgs(notification.DeliveryStateQueued, now.UTC(), 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"dedup_key",
			"event_id",
			"conversation_id",
			"message_id",
			"account_id",
			"device_id",
			"push_token_id",
			"kind",
			"reason",
			"mode",
			"state",
			"priority",
			"attempts",
			"next_attempt_at",
			"lease_token",
			"lease_expires_at",
			"last_attempt_at",
			"last_error",
			"created_at",
			"updated_at",
		}).
			AddRow("del-1", "dedup-1", "evt-1", "conv-1", "msg-1", "acc-1", "", "", notification.NotificationKindGroup, "group", notification.DeliveryModeInApp, notification.DeliveryStateQueued, 100, 0, now.UTC(), "", nil, nil, "", now.UTC(), now.UTC()).
			AddRow("del-2", "dedup-2", "evt-2", "conv-1", "msg-2", "acc-2", "", "", notification.NotificationKindGroup, "group", notification.DeliveryModeInApp, notification.DeliveryStateQueued, 50, 0, now.Add(5*time.Second).UTC(), "", nil, nil, "", now.UTC(), now.UTC()))

	deliveries, err := store.DeliveriesDue(context.Background(), now, 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 2)
	require.Equal(t, "del-1", deliveries[0].ID)
	require.Equal(t, "del-2", deliveries[1].ID)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDeliveryMonotonicUpsert(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "notif"\."notification_deliveries" AS existing .*ON CONFLICT \(dedup_key\) DO UPDATE SET.*WHEN EXCLUDED\.attempts > existing\.attempts THEN EXCLUDED\.state.*attempts = GREATEST\(existing\.attempts, EXCLUDED\.attempts\).*RETURNING.*lease_token.*lease_expires_at.*updated_at`).
		WithArgs(
			"del-1",
			"evt-1:conv-1:msg-1:acc-1::group:group",
			"evt-1",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			notification.NotificationKindGroup,
			"group",
			notification.DeliveryModeInApp,
			notification.DeliveryStateQueued,
			10,
			1,
			now.UTC(),
			"",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"",
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"dedup_key",
			"event_id",
			"conversation_id",
			"message_id",
			"account_id",
			"device_id",
			"push_token_id",
			"kind",
			"reason",
			"mode",
			"state",
			"priority",
			"attempts",
			"next_attempt_at",
			"lease_token",
			"lease_expires_at",
			"last_attempt_at",
			"last_error",
			"created_at",
			"updated_at",
		}).AddRow(
			"del-1",
			"evt-1:conv-1:msg-1:acc-1::group:group",
			"evt-1",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			notification.NotificationKindGroup,
			"group",
			notification.DeliveryModeInApp,
			notification.DeliveryStateQueued,
			10,
			2,
			now.UTC(),
			"",
			nil,
			nil,
			"",
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveDelivery(context.Background(), notification.Delivery{
		ID:             "del-1",
		DedupKey:       "evt-1:conv-1:msg-1:acc-1::group:group",
		EventID:        "evt-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		AccountID:      "acc-1",
		Kind:           notification.NotificationKindGroup,
		Reason:         "group",
		Mode:           notification.DeliveryModeInApp,
		State:          notification.DeliveryStateQueued,
		Priority:       10,
		Attempts:       1,
		NextAttemptAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	require.NoError(t, err)
	require.Equal(t, 2, saved.Attempts)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveWorkerCursorMonotonicUpsert(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "notif"\."notification_worker_cursors" AS existing .*ON CONFLICT \(name\) DO UPDATE SET.*WHEN EXCLUDED\.last_sequence > existing\.last_sequence THEN EXCLUDED\.updated_at.*RETURNING name, last_sequence, updated_at`).
		WithArgs("conversation_notifications", uint64(5), now.UTC()).
		WillReturnRows(sqlmock.NewRows([]string{
			"name",
			"last_sequence",
			"updated_at",
		}).AddRow(
			"conversation_notifications",
			uint64(10),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveWorkerCursor(context.Background(), notification.WorkerCursor{
		Name:         "conversation_notifications",
		LastSequence: 5,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(10), saved.LastSequence)

	require.NoError(t, mock.ExpectationsWereMet())
}
