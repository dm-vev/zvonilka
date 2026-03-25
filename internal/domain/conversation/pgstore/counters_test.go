package pgstore

import (
	"context"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

func TestConversationCountersIgnoreHistoryBeforeJoin(t *testing.T) {
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
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedIdentity(t, db, "tenant",
		"id-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
		"id-peer", "peer", "peer@example.com", "dev-peer", "sess-peer",
	)

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return time.Date(2026, time.March, 24, 17, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID: "id-owner",
		Kind:           conversation.ConversationKindGroup,
		Title:          "Group",
		CreatedAt:      time.Date(2026, time.March, 24, 17, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	firstMessage, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce-1"),
				Ciphertext: []byte("ciphertext-1"),
				AAD:        []byte("aad-1"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 17, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("send first message: %v", err)
	}

	if _, err := store.SaveConversationMember(context.Background(), conversation.ConversationMember{
		ConversationID: created.ID,
		AccountID:      "id-peer",
		Role:           conversation.MemberRoleMember,
		JoinedAt:       time.Date(2026, time.March, 24, 17, 2, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("save peer member: %v", err)
	}

	secondMessage, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind:              conversation.MessageKindText,
			MentionAccountIDs: []string{"id-peer"},
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-2",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce-2"),
				Ciphertext: []byte("ciphertext-2"),
				AAD:        []byte("aad-2"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 17, 3, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("send second message: %v", err)
	}

	conversations, err := svc.ListConversations(context.Background(), conversation.ListConversationsParams{
		AccountID: "id-peer",
	})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(conversations))
	}
	if conversations[0].UnreadCount != 1 {
		t.Fatalf("expected 1 unread message after join, got %d", conversations[0].UnreadCount)
	}
	if conversations[0].UnreadMentionCount != 1 {
		t.Fatalf("expected 1 unread mention after join, got %d", conversations[0].UnreadMentionCount)
	}
	if conversations[0].LastSequence != secondMessage.Sequence {
		t.Fatalf("expected last sequence %d, got %d", secondMessage.Sequence, conversations[0].LastSequence)
	}
	if firstMessage.Sequence >= secondMessage.Sequence {
		t.Fatalf("expected first message sequence to stay behind second message")
	}
}

func TestConversationCountersPersistMentions(t *testing.T) {
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
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedIdentity(t, db, "tenant",
		"id-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
		"id-peer", "peer", "peer@example.com", "dev-peer", "sess-peer",
	)

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	svc, err := conversation.NewService(store, conversation.WithNow(func() time.Time {
		return time.Date(2026, time.March, 24, 16, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID:   "id-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"id-peer"},
		CreatedAt:        time.Date(2026, time.March, 24, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	message, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			ClientMessageID:   "client-1",
			Kind:              conversation.MessageKindText,
			MentionAccountIDs: []string{"id-peer"},
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 16, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	loadedMessage, err := store.MessageByID(context.Background(), created.ID, message.ID)
	if err != nil {
		t.Fatalf("load message: %v", err)
	}
	if len(loadedMessage.MentionAccountIDs) != 1 || loadedMessage.MentionAccountIDs[0] != "id-peer" {
		t.Fatalf("expected persisted mention targets, got %#v", loadedMessage.MentionAccountIDs)
	}

	conversations, err := svc.ListConversations(context.Background(), conversation.ListConversationsParams{
		AccountID: "id-peer",
	})
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(conversations))
	}
	if conversations[0].UnreadCount != 1 {
		t.Fatalf("expected 1 unread message, got %d", conversations[0].UnreadCount)
	}
	if conversations[0].UnreadMentionCount != 1 {
		t.Fatalf("expected 1 unread mention, got %d", conversations[0].UnreadMentionCount)
	}

	if _, _, err := svc.MarkRead(context.Background(), conversation.MarkReadParams{
		ConversationID:      created.ID,
		AccountID:           "id-peer",
		DeviceID:            "dev-peer",
		ReadThroughSequence: message.Sequence,
		CreatedAt:           time.Date(2026, time.March, 24, 16, 2, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	conversations, err = svc.ListConversations(context.Background(), conversation.ListConversationsParams{
		AccountID: "id-peer",
	})
	if err != nil {
		t.Fatalf("list conversations after read: %v", err)
	}
	if conversations[0].UnreadCount != 0 {
		t.Fatalf("expected 0 unread messages after read, got %d", conversations[0].UnreadCount)
	}
	if conversations[0].UnreadMentionCount != 0 {
		t.Fatalf("expected 0 unread mentions after read, got %d", conversations[0].UnreadMentionCount)
	}
}
