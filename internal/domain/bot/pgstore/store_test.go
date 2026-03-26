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

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func newMockStore(t *testing.T) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	store, err := New(db, "bot")
	require.NoError(t, err)

	return store, mock, db
}

func TestMapConstraintError(t *testing.T) {
	t.Parallel()

	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23505"}), bot.ErrConflict))
	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23503"}), bot.ErrNotFound))
	require.True(t, errors.Is(mapConstraintError(&pgconn.PgError{Code: "23514"}), bot.ErrInvalidInput))
	require.Nil(t, mapConstraintError(&pgconn.PgError{Code: "99999"}))
}

func TestSaveWebhookRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 12, 0, 0, 0, time.UTC)
	allowed, err := encodeAllowedUpdates([]bot.UpdateType{bot.UpdateTypeMessage})
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "bot"\."bot_webhooks".*RETURNING bot_account_id, url, secret_token, allowed_updates, max_connections, last_error_message, last_error_at, last_success_at, created_at, updated_at`).
		WithArgs(
			"acc-bot",
			"https://example.org/hook",
			"secret",
			allowed,
			40,
			"",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"bot_account_id",
			"url",
			"secret_token",
			"allowed_updates",
			"max_connections",
			"last_error_message",
			"last_error_at",
			"last_success_at",
			"created_at",
			"updated_at",
		}).AddRow(
			"acc-bot",
			"https://example.org/hook",
			"secret",
			allowed,
			40,
			"",
			nil,
			nil,
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveWebhook(context.Background(), bot.Webhook{
		BotAccountID:   "acc-bot",
		URL:            "https://example.org/hook",
		SecretToken:    "secret",
		AllowedUpdates: []bot.UpdateType{bot.UpdateTypeMessage},
		MaxConnections: 40,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	require.NoError(t, err)
	require.Equal(t, "acc-bot", saved.BotAccountID)
	require.Equal(t, "https://example.org/hook", saved.URL)
	require.Equal(t, []bot.UpdateType{bot.UpdateTypeMessage}, saved.AllowedUpdates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveUpdateRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 12, 0, 0, 0, time.UTC)
	payload, err := encodeUpdate(bot.Update{
		Message: &bot.Message{
			MessageID: "msg-1",
			Text:      "hello",
		},
	})
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "bot"\."bot_updates".*RETURNING update_id, bot_account_id, event_id, update_type, payload, attempts, next_attempt_at, last_error, created_at, updated_at`).
		WithArgs(
			"acc-bot",
			"evt-1",
			bot.UpdateTypeMessage,
			payload,
			0,
			now.UTC(),
			"",
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"update_id",
			"bot_account_id",
			"event_id",
			"update_type",
			"payload",
			"attempts",
			"next_attempt_at",
			"last_error",
			"created_at",
			"updated_at",
		}).AddRow(
			int64(7),
			"acc-bot",
			"evt-1",
			bot.UpdateTypeMessage,
			payload,
			0,
			now.UTC(),
			"",
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveUpdate(context.Background(), bot.QueueEntry{
		BotAccountID:  "acc-bot",
		EventID:       "evt-1",
		UpdateType:    bot.UpdateTypeMessage,
		Payload:       bot.Update{Message: &bot.Message{MessageID: "msg-1", Text: "hello"}},
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	require.NoError(t, err)
	require.EqualValues(t, 7, saved.UpdateID)
	require.NotNil(t, saved.Payload.Message)
	require.Equal(t, "hello", saved.Payload.Message.Text)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveCursorMonotonicUpsert(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "bot"\."bot_worker_cursors".*GREATEST\(existing.last_sequence, EXCLUDED.last_sequence\).*RETURNING name, last_sequence, updated_at`).
		WithArgs("bot_updates", uint64(42), now.UTC()).
		WillReturnRows(sqlmock.NewRows([]string{
			"name",
			"last_sequence",
			"updated_at",
		}).AddRow("bot_updates", uint64(42), now.UTC()))
	mock.ExpectCommit()

	saved, err := store.SaveCursor(context.Background(), bot.Cursor{
		Name:         "bot_updates",
		LastSequence: 42,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	require.EqualValues(t, 42, saved.LastSequence)
	require.NoError(t, mock.ExpectationsWereMet())
}
