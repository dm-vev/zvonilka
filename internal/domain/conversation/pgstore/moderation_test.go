package pgstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

func TestModerationSchemaLifecycle(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t,
		"0001.sql",
		"0002_identity_hardening.sql",
		"0003_identity_account_boundaries.sql",
		"0004_identity_session_device_deferrable.sql",
		"0005.sql",
		"0006.sql",
		"0009.sql",
		"0010.sql",
		"0011.sql",
		"0012.sql",
		"0016.sql",
	)
	require.NoError(t, platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"))

	seedIdentity(t, db, "tenant",
		"acc-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
		"acc-peer", "peer", "peer@example.com", "dev-peer", "sess-peer",
	)

	store, err := New(db, "tenant")
	require.NoError(t, err)

	now := time.Date(2026, time.March, 25, 15, 0, 0, 0, time.UTC)

	policy, err := store.SaveModerationPolicy(context.Background(), conversation.ModerationPolicy{
		TargetKind:         conversation.ModerationTargetKindConversation,
		TargetID:           "conv-1",
		OnlyAdminsCanWrite: true,
		AllowReactions:     false,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	require.NoError(t, err)
	require.True(t, policy.OnlyAdminsCanWrite)

	loadedPolicy, err := store.ModerationPolicyByTarget(context.Background(), conversation.ModerationTargetKindConversation, "conv-1")
	require.NoError(t, err)
	require.Equal(t, policy.TargetID, loadedPolicy.TargetID)

	restriction, err := store.SaveModerationRestriction(context.Background(), conversation.ModerationRestriction{
		TargetKind:         conversation.ModerationTargetKindConversation,
		TargetID:           "conv-1",
		AccountID:          "acc-peer",
		State:              conversation.ModerationRestrictionStateShadowed,
		AppliedByAccountID: "acc-owner",
		Reason:             "shadow",
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	require.NoError(t, err)
	require.Equal(t, conversation.ModerationRestrictionStateShadowed, restriction.State)

	loadedRestriction, err := store.ModerationRestrictionByTargetAndAccount(
		context.Background(),
		conversation.ModerationTargetKindConversation,
		"conv-1",
		"acc-peer",
	)
	require.NoError(t, err)
	require.Equal(t, restriction.State, loadedRestriction.State)

	report, err := store.SaveModerationReport(context.Background(), conversation.ModerationReport{
		ID:                "rep-1",
		TargetKind:        conversation.ModerationTargetKindConversation,
		TargetID:          "conv-1",
		ReporterAccountID: "acc-peer",
		TargetAccountID:   "acc-owner",
		Reason:            "spam",
		Status:            conversation.ModerationReportStatusPending,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	require.NoError(t, err)
	require.Equal(t, conversation.ModerationReportStatusPending, report.Status)

	loadedReport, err := store.ModerationReportByID(context.Background(), report.ID)
	require.NoError(t, err)
	require.Equal(t, report.ID, loadedReport.ID)

	action, err := store.SaveModerationAction(context.Background(), conversation.ModerationAction{
		ID:              "mod-1",
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        "conv-1",
		ActorAccountID:  "acc-owner",
		TargetAccountID: "acc-peer",
		Type:            conversation.ModerationActionTypeShadowBan,
		Reason:          "shadow",
		Metadata: map[string]string{
			"source": "test",
		},
		CreatedAt: now,
	})
	require.NoError(t, err)
	require.Equal(t, "mod-1", action.ID)

	actions, err := store.ModerationActionsByTarget(context.Background(), conversation.ModerationTargetKindConversation, "conv-1")
	require.NoError(t, err)
	require.Len(t, actions, 1)

	rateState, err := store.SaveModerationRateState(context.Background(), conversation.ModerationRateState{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        "conv-1",
		AccountID:       "acc-peer",
		LastWriteAt:     now,
		WindowStartedAt: now,
		WindowCount:     2,
		UpdatedAt:       now,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), rateState.WindowCount)

	loadedRateState, err := store.ModerationRateStateByTargetAndAccount(
		context.Background(),
		conversation.ModerationTargetKindConversation,
		"conv-1",
		"acc-peer",
	)
	require.NoError(t, err)
	require.Equal(t, rateState.WindowCount, loadedRateState.WindowCount)
}

func TestModerationSchemaConstraints(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t,
		"0001.sql",
		"0002_identity_hardening.sql",
		"0003_identity_account_boundaries.sql",
		"0004_identity_session_device_deferrable.sql",
		"0005.sql",
		"0006.sql",
		"0009.sql",
		"0010.sql",
		"0011.sql",
		"0012.sql",
		"0016.sql",
	)
	require.NoError(t, platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"))

	seedIdentity(t, db, "tenant",
		"acc-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
	)

	_, err := db.ExecContext(context.Background(), `
INSERT INTO tenant.conversation_moderation_policies (
	target_kind, target_id, created_at, updated_at
) VALUES (
	$1, $2, $3, $4
)`,
		"alien",
		"conv-1",
		time.Now().UTC(),
		time.Now().UTC(),
	)
	require.Error(t, err)
}
