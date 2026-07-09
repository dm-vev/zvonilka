package teststore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func TestProfileAndRightsRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore()
	now := time.Date(2026, time.March, 26, 16, 0, 0, 0, time.UTC)

	profile, err := store.SaveProfile(ctx, bot.ProfileValue{
		BotAccountID: "bot-1",
		Kind:         bot.ProfileKindDescription,
		LanguageCode: "RU",
		Value:        "Описание",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	require.Equal(t, "ru", profile.LanguageCode)

	loadedProfile, err := store.ProfileByLanguage(ctx, "bot-1", bot.ProfileKindDescription, "ru")
	require.NoError(t, err)
	require.Equal(t, "Описание", loadedProfile.Value)

	err = store.DeleteProfile(ctx, "bot-1", bot.ProfileKindDescription, "ru")
	require.NoError(t, err)

	_, err = store.ProfileByLanguage(ctx, "bot-1", bot.ProfileKindDescription, "ru")
	require.ErrorIs(t, err, bot.ErrNotFound)

	rights, err := store.SaveRights(ctx, bot.AdminRightsState{
		BotAccountID: "bot-1",
		ForChannels:  true,
		Rights: bot.AdminRights{
			CanManageChat:     true,
			CanPostMessages:   true,
			CanDeleteMessages: true,
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	require.NoError(t, err)
	require.True(t, rights.Rights.CanPostMessages)

	loadedRights, err := store.RightsByScope(ctx, "bot-1", true)
	require.NoError(t, err)
	require.True(t, loadedRights.Rights.CanManageChat)

	err = store.DeleteRights(ctx, "bot-1", true)
	require.NoError(t, err)

	_, err = store.RightsByScope(ctx, "bot-1", true)
	require.ErrorIs(t, err, bot.ErrNotFound)
}
