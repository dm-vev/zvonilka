package teststore

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/bot"
)

func TestWithinTxRetriesWithoutLosingConcurrentWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore().(*memoryStore)

	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once

	var (
		wg    sync.WaitGroup
		txErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		txErr = store.WithinTx(ctx, func(tx bot.Store) error {
			enteredOnce.Do(func() {
				close(entered)
			})
			<-release

			_, err := tx.SaveCursor(ctx, bot.Cursor{
				Name:         "fanout",
				LastSequence: 42,
				UpdatedAt:    time.Now().UTC(),
			})

			return err
		})
	}()

	<-entered

	_, err := store.SaveInlineQuery(ctx, bot.InlineQueryState{
		ID:            "inq_1",
		BotAccountID:  "bot_1",
		FromAccountID: "acc_1",
		Query:         "docs",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})
	require.NoError(t, err)

	close(release)
	wg.Wait()
	require.NoError(t, txErr)

	inlineQuery, err := store.InlineQueryByID(ctx, "inq_1")
	require.NoError(t, err)
	require.Equal(t, "docs", inlineQuery.Query)

	cursor, err := store.CursorByName(ctx, "fanout")
	require.NoError(t, err)
	require.Equal(t, uint64(42), cursor.LastSequence)
}
