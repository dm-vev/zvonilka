package rtc

import (
	"context"
	"fmt"
	"os"
	"testing"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/stretchr/testify/require"
)

func TestClusterMigrationSoak(t *testing.T) {
	if os.Getenv("RUN_RTC_CLUSTER_SOAK") != "1" {
		t.Skip("set RUN_RTC_CLUSTER_SOAK=1 to run distributed rtc soak test")
	}

	harness := newBenchmarkClusterHarness(t, 52000, 52999)
	defer harness.Close(t)

	ctx := context.Background()

	const sessions = 100
	for i := range sessions {
		callID := fmt.Sprintf("cluster-soak-call-%d", i)
		sourceSessionID := "node-b:rtc_" + callID

		session, err := harness.cluster.EnsureSession(ctx, domaincall.Call{
			ID:              callID,
			ConversationID:  "cluster-soak-conv-" + callID,
			ActiveSessionID: sourceSessionID,
		})
		require.NoError(t, err)

		for _, participant := range []domaincall.RuntimeParticipant{
			{
				CallID:    callID,
				AccountID: "acc-a",
				DeviceID:  "dev-a",
				WithVideo: true,
			},
			{
				CallID:    callID,
				AccountID: "acc-b",
				DeviceID:  "dev-b",
				WithVideo: true,
			},
		} {
			_, err = harness.cluster.JoinSession(ctx, session.SessionID, participant)
			require.NoError(t, err)
		}

		migrated, err := harness.cluster.MigrateSession(ctx, domaincall.Call{
			ID:              callID,
			ConversationID:  "cluster-soak-conv-" + callID,
			ActiveSessionID: session.SessionID,
		})
		require.NoError(t, err)

		stats, err := harness.cluster.SessionStats(ctx, migrated.SessionID)
		require.NoError(t, err)
		require.Len(t, stats, 2)

		require.NoError(t, harness.cluster.CloseSession(ctx, migrated.SessionID))
		if session.SessionID != migrated.SessionID {
			require.NoError(t, harness.cluster.CloseSession(ctx, session.SessionID))
		}
	}
}
