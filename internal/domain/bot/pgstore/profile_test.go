package pgstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func TestSaveProfileRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 16, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "bot"\."bot_profiles".*RETURNING bot_account_id, profile_kind, language_code, value, created_at, updated_at`).
		WithArgs(
			"acc-bot",
			bot.ProfileKindName,
			"ru",
			"Помощник",
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"bot_account_id",
			"profile_kind",
			"language_code",
			"value",
			"created_at",
			"updated_at",
		}).AddRow(
			"acc-bot",
			bot.ProfileKindName,
			"ru",
			"Помощник",
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveProfile(context.Background(), bot.ProfileValue{
		BotAccountID: "acc-bot",
		Kind:         bot.ProfileKindName,
		LanguageCode: "RU",
		Value:        "Помощник",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	require.Equal(t, "ru", saved.LanguageCode)
	require.Equal(t, "Помощник", saved.Value)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveRightsRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 16, 0, 0, 0, time.UTC)
	rights := bot.AdminRights{
		CanManageChat:     true,
		CanDeleteMessages: true,
		CanPostMessages:   true,
	}
	raw, err := json.Marshal(rights)
	require.NoError(t, err)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "bot"\."bot_default_admin_rights".*RETURNING bot_account_id, for_channels, rights, created_at, updated_at`).
		WithArgs(
			"acc-bot",
			true,
			raw,
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"bot_account_id",
			"for_channels",
			"rights",
			"created_at",
			"updated_at",
		}).AddRow(
			"acc-bot",
			true,
			raw,
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveRights(context.Background(), bot.AdminRightsState{
		BotAccountID: "acc-bot",
		ForChannels:  true,
		Rights:       rights,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	require.True(t, saved.Rights.CanPostMessages)
	require.NoError(t, mock.ExpectationsWereMet())
}
