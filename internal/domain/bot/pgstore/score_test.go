package pgstore

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func TestSaveScoreRoundTrip(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 18, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)INSERT INTO "bot"\."bot_game_scores".*RETURNING bot_account_id, message_id, account_id, score, created_at, updated_at`).
		WithArgs(
			"acc-bot",
			"msg-1",
			"acc-user",
			99,
			now.UTC(),
			now.UTC(),
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"bot_account_id",
			"message_id",
			"account_id",
			"score",
			"created_at",
			"updated_at",
		}).AddRow(
			"acc-bot",
			"msg-1",
			"acc-user",
			99,
			now.UTC(),
			now.UTC(),
		))
	mock.ExpectCommit()

	saved, err := store.SaveScore(context.Background(), bot.GameScore{
		BotAccountID: "acc-bot",
		MessageID:    "msg-1",
		AccountID:    "acc-user",
		Score:        99,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	require.Equal(t, 99, saved.Score)
	require.Equal(t, "acc-user", saved.AccountID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScoreLookupAndList(t *testing.T) {
	t.Parallel()

	store, mock, db := newMockStore(t)
	defer db.Close()

	now := time.Date(2026, time.March, 26, 18, 30, 0, 0, time.UTC)
	mock.ExpectQuery(`(?s)SELECT bot_account_id, message_id, account_id, score, created_at, updated_at\s+FROM "bot"\."bot_game_scores"\s+WHERE bot_account_id = \$1 AND message_id = \$2 AND account_id = \$3`).
		WithArgs("acc-bot", "msg-1", "acc-user").
		WillReturnRows(sqlmock.NewRows([]string{
			"bot_account_id",
			"message_id",
			"account_id",
			"score",
			"created_at",
			"updated_at",
		}).AddRow(
			"acc-bot",
			"msg-1",
			"acc-user",
			21,
			now.UTC(),
			now.UTC(),
		))

	score, err := store.ScoreByMessageAndAccount(context.Background(), "acc-bot", "msg-1", "acc-user")
	require.NoError(t, err)
	require.Equal(t, 21, score.Score)

	mock.ExpectQuery(`(?s)SELECT bot_account_id, message_id, account_id, score, created_at, updated_at\s+FROM "bot"\."bot_game_scores"\s+WHERE bot_account_id = \$1 AND message_id = \$2\s+ORDER BY score DESC, updated_at ASC, account_id ASC`).
		WithArgs("acc-bot", "msg-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"bot_account_id",
			"message_id",
			"account_id",
			"score",
			"created_at",
			"updated_at",
		}).AddRow(
			"acc-bot",
			"msg-1",
			"acc-peer",
			42,
			now.UTC(),
			now.UTC(),
		).AddRow(
			"acc-bot",
			"msg-1",
			"acc-user",
			21,
			now.UTC(),
			now.UTC(),
		))

	scores, err := store.ListScoresByMessage(context.Background(), "acc-bot", "msg-1")
	require.NoError(t, err)
	require.Len(t, scores, 2)
	require.Equal(t, "acc-peer", scores[0].AccountID)
	require.Equal(t, 42, scores[0].Score)
	require.Equal(t, "acc-user", scores[1].AccountID)
	require.NoError(t, mock.ExpectationsWereMet())
}
