package conversation_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	teststore "github.com/dm-vev/zvonilka/internal/domain/conversation/teststore"
)

func TestModerationPolicyBlocksNonAdminWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 10, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	require.NoError(t, err)

	_, err = svc.SetModerationPolicy(ctx, conversation.SetModerationPolicyParams{
		TargetKind:         conversation.ModerationTargetKindConversation,
		TargetID:           created.ID,
		ActorAccountID:     "acc-owner",
		OnlyAdminsCanWrite: true,
		AllowReactions:     true,
		AllowForwards:      true,
		AllowThreads:       false,
		CreatedAt:          now,
	})
	require.NoError(t, err)

	_, _, err = svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-peer",
		SenderDeviceID:  "dev-peer",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(time.Minute),
	})
	require.ErrorIs(t, err, conversation.ErrForbidden)

	_, _, err = svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(2 * time.Minute),
	})
	require.NoError(t, err)
}

func TestModerationPolicyBlocksReactions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 10, 30, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	require.NoError(t, err)

	_, err = svc.SetModerationPolicy(ctx, conversation.SetModerationPolicyParams{
		TargetKind:     conversation.ModerationTargetKindConversation,
		TargetID:       created.ID,
		ActorAccountID: "acc-owner",
		AllowReactions: false,
		CreatedAt:      now,
	})
	require.NoError(t, err)

	message, _, err := svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(time.Minute),
	})
	require.NoError(t, err)

	_, _, err = svc.AddMessageReaction(ctx, conversation.AddMessageReactionParams{
		ConversationID: created.ID,
		MessageID:      message.ID,
		ActorAccountID: "acc-peer",
		ActorDeviceID:  "dev-peer",
		Reaction:       "👍",
		CreatedAt:      now.Add(2 * time.Minute),
	})
	require.ErrorIs(t, err, conversation.ErrForbidden)
}

func TestModerationPolicyBlocksReactionRemoval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 10, 35, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	require.NoError(t, err)

	_, err = svc.SetModerationPolicy(ctx, conversation.SetModerationPolicyParams{
		TargetKind:     conversation.ModerationTargetKindConversation,
		TargetID:       created.ID,
		ActorAccountID: "acc-owner",
		AllowReactions: false,
		CreatedAt:      now,
	})
	require.NoError(t, err)

	message, _, err := svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(time.Minute),
	})
	require.NoError(t, err)

	_, _, err = svc.RemoveMessageReaction(ctx, conversation.RemoveMessageReactionParams{
		ConversationID: created.ID,
		MessageID:      message.ID,
		ActorAccountID: "acc-peer",
		ActorDeviceID:  "dev-peer",
		Reaction:       "👍",
		RemovedAt:      now.Add(2 * time.Minute),
	})
	require.ErrorIs(t, err, conversation.ErrForbidden)
}

func TestModerationPolicyRestrictsPinsToAdmins(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 10, 45, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	require.NoError(t, err)

	_, err = svc.SetModerationPolicy(ctx, conversation.SetModerationPolicyParams{
		TargetKind:               conversation.ModerationTargetKindConversation,
		TargetID:                 created.ID,
		ActorAccountID:           "acc-owner",
		PinnedMessagesOnlyAdmins: true,
		CreatedAt:                now,
	})
	require.NoError(t, err)

	message, _, err := svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(time.Minute),
	})
	require.NoError(t, err)

	_, _, err = svc.PinMessage(ctx, conversation.PinMessageParams{
		ConversationID: created.ID,
		MessageID:      message.ID,
		ActorAccountID: "acc-peer",
		ActorDeviceID:  "dev-peer",
		Pinned:         true,
		UpdatedAt:      now.Add(2 * time.Minute),
	})
	require.ErrorIs(t, err, conversation.ErrForbidden)
}

func TestModerationRestrictionBlocksMutedWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 11, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	require.NoError(t, err)

	_, err = svc.ApplyModerationRestriction(ctx, conversation.ApplyModerationRestrictionParams{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        created.ID,
		ActorAccountID:  "acc-owner",
		TargetAccountID: "acc-peer",
		State:           conversation.ModerationRestrictionStateMuted,
		Reason:          "spam",
		CreatedAt:       now,
	})
	require.NoError(t, err)

	_, _, err = svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-peer",
		SenderDeviceID:  "dev-peer",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(time.Minute),
	})
	require.ErrorIs(t, err, conversation.ErrForbidden)
}

func TestShadowRestrictionHidesMessagesFromOtherMembers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	require.NoError(t, err)

	_, err = svc.ApplyModerationRestriction(ctx, conversation.ApplyModerationRestrictionParams{
		TargetKind:      conversation.ModerationTargetKindConversation,
		TargetID:        created.ID,
		ActorAccountID:  "acc-owner",
		TargetAccountID: "acc-peer",
		State:           conversation.ModerationRestrictionStateShadowed,
		Reason:          "shadow",
		CreatedAt:       now,
	})
	require.NoError(t, err)

	_, _, err = svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-peer",
		SenderDeviceID:  "dev-peer",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(time.Minute),
	})
	require.NoError(t, err)

	ownerMessages, err := svc.ListMessages(ctx, conversation.ListMessagesParams{
		AccountID:      "acc-owner",
		ConversationID: created.ID,
	})
	require.NoError(t, err)
	require.Empty(t, ownerMessages)

	peerMessages, err := svc.ListMessages(ctx, conversation.ListMessagesParams{
		AccountID:      "acc-peer",
		ConversationID: created.ID,
	})
	require.NoError(t, err)
	require.Len(t, peerMessages, 1)
}

func TestModerationSlowModeRejectsRapidWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 13, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	require.NoError(t, err)

	_, err = svc.SetModerationPolicy(ctx, conversation.SetModerationPolicyParams{
		TargetKind:       conversation.ModerationTargetKindConversation,
		TargetID:         created.ID,
		ActorAccountID:   "acc-owner",
		SlowModeInterval: time.Hour,
		CreatedAt:        now,
	})
	require.NoError(t, err)

	_, _, err = svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(time.Minute),
	})
	require.NoError(t, err)

	_, _, err = svc.SendMessage(ctx, conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "acc-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: now.Add(2 * time.Minute),
	})
	require.ErrorIs(t, err, conversation.ErrRateLimited)
}

func TestModerationReportLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := teststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 14, 0, 0, 0, time.UTC)

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return now
	}))
	require.NoError(t, err)

	created, _, err := svc.CreateConversation(ctx, conversation.CreateConversationParams{
		OwnerAccountID:   "acc-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"acc-peer"},
		CreatedAt:        now,
	})
	require.NoError(t, err)

	report, err := svc.SubmitModerationReport(ctx, conversation.SubmitModerationReportParams{
		TargetKind:        conversation.ModerationTargetKindConversation,
		TargetID:          created.ID,
		ReporterAccountID: "acc-peer",
		TargetAccountID:   "acc-owner",
		Reason:            "spam",
		Details:           "too many messages",
		CreatedAt:         now,
	})
	require.NoError(t, err)
	require.Equal(t, conversation.ModerationReportStatusPending, report.Status)

	resolved, err := svc.ResolveModerationReport(ctx, conversation.ResolveModerationReportParams{
		ReportID:          report.ID,
		ResolverAccountID: "acc-owner",
		Resolved:          true,
		Resolution:        "reviewed",
		ReviewedAt:        now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.Equal(t, conversation.ModerationReportStatusResolved, resolved.Status)
	require.Equal(t, "acc-owner", resolved.ReviewedByAccountID)
}
