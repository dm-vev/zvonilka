package teststore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func TestSaveScoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	now := time.Date(2026, time.March, 26, 12, 0, 0, 0, time.UTC)

	saved, err := store.SaveScore(context.Background(), bot.GameScore{
		BotAccountID: "bot_1",
		MessageID:    "msg_1",
		AccountID:    "acc_1",
		Score:        42,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	require.Equal(t, 42, saved.Score)

	loaded, err := store.ScoreByMessageAndAccount(context.Background(), "bot_1", "msg_1", "acc_1")
	require.NoError(t, err)
	require.Equal(t, 42, loaded.Score)

	list, err := store.ListScoresByMessage(context.Background(), "bot_1", "msg_1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "acc_1", list[0].AccountID)
}
