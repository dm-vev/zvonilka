package pgstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	"github.com/dm-vev/zvonilka/internal/testsupport/dockermutex"
)

func TestConversationSchemaLifecycle(t *testing.T) {
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
		"0018.sql",
		"0019.sql",
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
		return time.Date(2026, time.March, 24, 11, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, members, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID:   "id-owner",
		Kind:             conversation.ConversationKindDirect,
		Title:            "Direct",
		MemberAccountIDs: []string{"id-peer"},
		CreatedAt:        time.Date(2026, time.March, 24, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	message, event, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			ClientMessageID: "client-1",
			Kind:            conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
				AAD:        []byte("aad"),
				Metadata:   map[string]string{"format": "v1"},
			},
			Attachments: []conversation.AttachmentRef{
				{
					MediaID:   "media-1",
					Kind:      conversation.AttachmentKindImage,
					FileName:  "photo.jpg",
					MimeType:  "image/jpeg",
					SizeBytes: 1024,
					SHA256Hex: "abc123",
					Caption:   "plaintext caption",
				},
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 11, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	if message.Sequence != event.Sequence {
		t.Fatalf("expected matching message/event sequence, got %d and %d", message.Sequence, event.Sequence)
	}
	if len(message.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(message.Attachments))
	}

	if _, _, err := svc.RecordDelivery(context.Background(), conversation.RecordDeliveryParams{
		ConversationID:           created.ID,
		AccountID:                "id-peer",
		DeviceID:                 "dev-peer",
		MessageID:                message.ID,
		DeliveredThroughSequence: event.Sequence,
		CreatedAt:                time.Date(2026, time.March, 24, 11, 2, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("record delivery: %v", err)
	}

	readState, _, err := svc.MarkRead(context.Background(), conversation.MarkReadParams{
		ConversationID:      created.ID,
		AccountID:           "id-peer",
		DeviceID:            "dev-peer",
		ReadThroughSequence: event.Sequence,
		CreatedAt:           time.Date(2026, time.March, 24, 11, 3, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if readState.LastReadSequence != event.Sequence {
		t.Fatalf("expected read watermark %d, got %d", event.Sequence, readState.LastReadSequence)
	}

	loadedConversation, err := store.ConversationByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("load conversation: %v", err)
	}
	if loadedConversation.LastSequence != event.Sequence {
		t.Fatalf("expected last sequence %d, got %d", event.Sequence, loadedConversation.LastSequence)
	}
	if loadedConversation.Settings.RequireEncryptedMessages {
		t.Fatal("expected encrypted message policy to remain opt-in by default")
	}

	loadedMessage, err := store.MessageByID(context.Background(), created.ID, message.ID)
	if err != nil {
		t.Fatalf("load message: %v", err)
	}
	if len(loadedMessage.Attachments) != 1 {
		t.Fatalf("expected loaded attachment, got %d", len(loadedMessage.Attachments))
	}
	if loadedMessage.ReplyTo.MessageID != "" || loadedMessage.ReplyTo.ConversationID != "" {
		t.Fatalf("expected root message without reply reference, got %+v", loadedMessage.ReplyTo)
	}
	if loadedMessage.Attachments[0].Caption != "" {
		t.Fatalf("expected attachment caption to be stripped, got %q", loadedMessage.Attachments[0].Caption)
	}

	events, syncState, err := svc.PullEvents(context.Background(), conversation.PullEventsParams{
		DeviceID:     "dev-peer",
		FromSequence: 0,
		Limit:        100,
	})
	if err != nil {
		t.Fatalf("pull events: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}
	if events[0].EventType != conversation.EventTypeConversationCreated {
		t.Fatalf("expected first event to be conversation created, got %s", events[0].EventType)
	}
	if events[0].MessageID != "" {
		t.Fatalf("expected conversation-created event without message id, got %q", events[0].MessageID)
	}
	if syncState.LastAppliedSequence != events[len(events)-1].Sequence {
		t.Fatalf("sync state did not advance to latest event")
	}
}

func TestConversationSchemaConstraints(t *testing.T) {
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
		"0018.sql",
		"0019.sql",
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedIdentity(t, db, "tenant",
		"id-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
	)

	_, err := db.ExecContext(context.Background(), `
INSERT INTO tenant.conversation_conversations (
	id, kind, title, description, avatar_media_id, owner_account_id,
	only_admins_can_write, only_admins_can_add_members, allow_reactions, allow_forwards,
	allow_threads, require_join_approval, pinned_messages_only_admins, slow_mode_interval_nanos,
	archived, muted, pinned, hidden, last_sequence, created_at, updated_at, last_message_at
) VALUES (
	$1, $2, $3, $4, $5, $6,
	$7, $8, $9, $10,
	$11, $12, $13, $14,
	$15, $16, $17, $18, $19, $20, $21, $22
)`,
		"conv-invalid",
		"alien",
		"Test",
		"",
		"",
		"id-owner",
		false, false, true, true, true, false, false, 0,
		false, false, false, false, 0,
		time.Now().UTC(),
		time.Now().UTC(),
		nil,
	)
	if err == nil {
		t.Fatal("expected invalid conversation kind to fail")
	}
}

func TestModerationPolicySchemaRejectsAntiSpamMismatch(t *testing.T) {
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
		"0018.sql",
		"0019.sql",
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	_, err := db.ExecContext(context.Background(), `
INSERT INTO tenant.conversation_moderation_policies (
	target_kind, target_id, anti_spam_window_nanos, anti_spam_burst_limit, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6
)`,
		"conversation",
		"conv-anti-spam",
		time.Hour.Nanoseconds(),
		0,
		time.Now().UTC(),
		time.Now().UTC(),
	)
	if err == nil {
		t.Fatal("expected anti-spam mismatch to fail")
	}
}

func TestModerationReportSchemaRejectsReviewMismatch(t *testing.T) {
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
		"0018.sql",
		"0019.sql",
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedIdentity(t, db, "tenant",
		"acc-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
	)

	_, err := db.ExecContext(context.Background(), `
INSERT INTO tenant.conversation_moderation_reports (
	id, target_kind, target_id, reporter_account_id, target_account_id, reason, status, reviewed_by_account_id, reviewed_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)`,
		"rep-invalid",
		"conversation",
		"conv-1",
		"acc-owner",
		"acc-owner",
		"spam",
		"resolved",
		"",
		nil,
		time.Now().UTC(),
		time.Now().UTC(),
	)
	if err == nil {
		t.Fatal("expected review mismatch to fail")
	}
}

func TestConversationTopicSchemaLifecycle(t *testing.T) {
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
		"0018.sql",
		"0019.sql",
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
		return time.Date(2026, time.March, 24, 13, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID:   "id-owner",
		Kind:             conversation.ConversationKindGroup,
		Title:            "Group",
		MemberAccountIDs: []string{"id-peer"},
		CreatedAt:        time.Date(2026, time.March, 24, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	topics, err := svc.ListTopics(context.Background(), conversation.ListTopicsParams{
		ConversationID: created.ID,
		AccountID:      "id-owner",
	})
	if err != nil {
		t.Fatalf("list topics: %v", err)
	}
	if len(topics) != 1 || !topics[0].IsGeneral {
		t.Fatalf("expected general root topic, got %+v", topics)
	}

	rootMessage, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-root",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 13, 0, 30, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("send root message: %v", err)
	}

	rootTopic, err := store.TopicByConversationAndID(context.Background(), created.ID, "")
	if err != nil {
		t.Fatalf("load root topic: %v", err)
	}
	if rootTopic.MessageCount != 1 || rootTopic.LastSequence != rootMessage.Sequence {
		t.Fatalf("expected root topic counters to advance, got %+v", rootTopic)
	}
	rootMessages, err := svc.ListMessages(context.Background(), conversation.ListMessagesParams{
		AccountID:      "id-owner",
		ConversationID: created.ID,
	})
	if err != nil {
		t.Fatalf("list root messages: %v", err)
	}
	if len(rootMessages) != 1 || rootMessages[0].ID != rootMessage.ID {
		t.Fatalf("expected root message to be listed, got %+v", rootMessages)
	}

	topic, _, err := svc.CreateTopic(context.Background(), conversation.CreateTopicParams{
		ConversationID:   created.ID,
		RootMessageID:    rootMessage.ID,
		CreatorAccountID: "id-owner",
		Title:            "Announcements",
		CreatedAt:        time.Date(2026, time.March, 24, 13, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create topic: %v", err)
	}
	if topic.RootMessageID != rootMessage.ID {
		t.Fatalf("expected root message id %s, got %s", rootMessage.ID, topic.RootMessageID)
	}

	renamed, _, err := svc.RenameTopic(context.Background(), conversation.RenameTopicParams{
		ConversationID: created.ID,
		TopicID:        topic.ID,
		ActorAccountID: "id-owner",
		Title:          "Updates",
		UpdatedAt:      time.Date(2026, time.March, 24, 13, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("rename topic: %v", err)
	}
	if renamed.Title != "Updates" {
		t.Fatalf("expected renamed topic, got %q", renamed.Title)
	}

	message, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind:     conversation.MessageKindText,
			ThreadID: topic.ID,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-1",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 13, 3, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("send topic message: %v", err)
	}

	threadMessages, err := svc.ListMessages(context.Background(), conversation.ListMessagesParams{
		AccountID:      "id-owner",
		ConversationID: created.ID,
		ThreadID:       topic.ID,
	})
	if err != nil {
		t.Fatalf("list topic messages: %v", err)
	}
	if len(threadMessages) != 1 || threadMessages[0].ID != message.ID {
		t.Fatalf("expected one topic message, got %+v", threadMessages)
	}

	archived, _, err := svc.ArchiveTopic(context.Background(), conversation.ArchiveTopicParams{
		ConversationID: created.ID,
		TopicID:        topic.ID,
		ActorAccountID: "id-owner",
		Archived:       true,
		UpdatedAt:      time.Date(2026, time.March, 24, 13, 4, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("archive topic: %v", err)
	}
	if !archived.Archived {
		t.Fatalf("expected archived topic")
	}

	_, _, err = svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind:     conversation.MessageKindText,
			ThreadID: topic.ID,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-2",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 13, 5, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected archived topic to reject writes")
	}

	if _, err := store.SaveMessage(context.Background(), conversation.Message{
		ID:              "msg-fk",
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Kind:            conversation.MessageKindText,
		Status:          conversation.MessageStatusSent,
		ThreadID:        "missing-topic",
		Payload: conversation.EncryptedPayload{
			KeyID:      "key-3",
			Algorithm:  "xchacha20poly1305",
			Nonce:      []byte("nonce"),
			Ciphertext: []byte("ciphertext"),
		},
		CreatedAt: time.Date(2026, time.March, 24, 13, 6, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.March, 24, 13, 6, 0, 0, time.UTC),
	}); err == nil {
		t.Fatal("expected missing topic fk to fail")
	}
}

func TestConversationMessageActionSchemaLifecycle(t *testing.T) {
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
		"0018.sql",
		"0019.sql",
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
		return time.Date(2026, time.March, 24, 14, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID:   "id-owner",
		Kind:             conversation.ConversationKindGroup,
		Title:            "Group",
		MemberAccountIDs: []string{"id-peer"},
		Settings: conversation.ConversationSettings{
			AllowReactions:           true,
			PinnedMessagesOnlyAdmins: true,
		},
		CreatedAt: time.Date(2026, time.March, 24, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	rootMessage, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			ClientMessageID: "root-1",
			Kind:            conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-root",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 14, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("send root message: %v", err)
	}

	replyMessage, _, err := svc.ReplyMessage(context.Background(), conversation.ReplyMessageParams{
		ConversationID:   created.ID,
		SenderAccountID:  "id-peer",
		SenderDeviceID:   "dev-peer",
		ReplyToMessageID: rootMessage.ID,
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-reply",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 14, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("reply message: %v", err)
	}
	if replyMessage.ReplyTo.MessageID != rootMessage.ID {
		t.Fatalf("expected reply reference, got %+v", replyMessage.ReplyTo)
	}

	edited, _, err := svc.EditMessage(context.Background(), conversation.EditMessageParams{
		ConversationID: created.ID,
		MessageID:      replyMessage.ID,
		ActorAccountID: "id-peer",
		ActorDeviceID:  "dev-peer",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				KeyID:      "key-reply-2",
				Algorithm:  "xchacha20poly1305",
				Nonce:      []byte("nonce"),
				Ciphertext: []byte("ciphertext"),
			},
		},
		EditedAt: time.Date(2026, time.March, 24, 14, 3, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("edit message: %v", err)
	}
	if edited.EditedAt.IsZero() {
		t.Fatal("expected edited message timestamp")
	}

	replied, _, err := svc.AddMessageReaction(context.Background(), conversation.AddMessageReactionParams{
		ConversationID: created.ID,
		MessageID:      replyMessage.ID,
		ActorAccountID: "id-owner",
		ActorDeviceID:  "dev-owner",
		Reaction:       "👍",
		CreatedAt:      time.Date(2026, time.March, 24, 14, 4, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("add reaction: %v", err)
	}
	if len(replied.Reactions) != 1 {
		t.Fatalf("expected one reaction, got %+v", replied.Reactions)
	}

	removed, _, err := svc.RemoveMessageReaction(context.Background(), conversation.RemoveMessageReactionParams{
		ConversationID: created.ID,
		MessageID:      replyMessage.ID,
		ActorAccountID: "id-owner",
		ActorDeviceID:  "dev-owner",
		Reaction:       "👍",
		RemovedAt:      time.Date(2026, time.March, 24, 14, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("remove reaction: %v", err)
	}
	if len(removed.Reactions) != 0 {
		t.Fatalf("expected reaction removal, got %+v", removed.Reactions)
	}

	_, _, err = svc.PinMessage(context.Background(), conversation.PinMessageParams{
		ConversationID: created.ID,
		MessageID:      replyMessage.ID,
		ActorAccountID: "id-peer",
		ActorDeviceID:  "dev-peer",
		Pinned:         true,
		UpdatedAt:      time.Date(2026, time.March, 24, 14, 6, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected non-admin pin to fail")
	}

	pinned, _, err := svc.PinMessage(context.Background(), conversation.PinMessageParams{
		ConversationID: created.ID,
		MessageID:      replyMessage.ID,
		ActorAccountID: "id-owner",
		ActorDeviceID:  "dev-owner",
		Pinned:         true,
		UpdatedAt:      time.Date(2026, time.March, 24, 14, 7, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("pin message: %v", err)
	}
	if !pinned.Pinned {
		t.Fatal("expected pinned message")
	}

	deleted, _, err := svc.DeleteMessage(context.Background(), conversation.DeleteMessageParams{
		ConversationID: created.ID,
		MessageID:      replyMessage.ID,
		ActorAccountID: "id-peer",
		ActorDeviceID:  "dev-peer",
		DeletedAt:      time.Date(2026, time.March, 24, 14, 8, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("delete message: %v", err)
	}
	if deleted.Status != conversation.MessageStatusDeleted {
		t.Fatalf("expected deleted message, got %+v", deleted)
	}

	messages, err := svc.ListMessages(context.Background(), conversation.ListMessagesParams{
		AccountID:      "id-owner",
		ConversationID: created.ID,
	})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != rootMessage.ID {
		t.Fatalf("expected deleted message to be hidden, got %+v", messages)
	}

	_, err = db.ExecContext(context.Background(), `
INSERT INTO tenant.conversation_message_reactions (
	message_id, account_id, reaction, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5)
`,
		rootMessage.ID,
		"id-owner",
		"",
		time.Date(2026, time.March, 24, 14, 9, 0, 0, time.UTC),
		time.Date(2026, time.March, 24, 14, 9, 0, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("expected invalid reaction insert to fail")
	}
}

func seedIdentity(t *testing.T, db *sql.DB, schema string, ownerID, ownerUsername, ownerEmail, ownerDeviceID, ownerSessionID string, peer ...string) {
	t.Helper()

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin seed tx: %v", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Date(2026, time.March, 24, 10, 59, 0, 0, time.UTC)
	accounts := []struct {
		id        string
		username  string
		email     string
		deviceID  string
		sessionID string
	}{
		{ownerID, ownerUsername, ownerEmail, ownerDeviceID, ownerSessionID},
	}
	if len(peer) >= 5 {
		accounts = append(accounts, struct {
			id        string
			username  string
			email     string
			deviceID  string
			sessionID string
		}{
			id:        peer[0],
			username:  peer[1],
			email:     peer[2],
			deviceID:  peer[3],
			sessionID: peer[4],
		})
	}
	for _, account := range accounts {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s.identity_accounts (
	id, kind, username, display_name, bio, email, phone, roles, status, bot_token_hash,
	created_by, created_at, updated_at, disabled_at, last_auth_at, custom_badge_emoji
) VALUES ($1, 'user', $2, $3, '', $4, '', '[]', 'active', '', 'seed', $5, $5, NULL, NULL, '')
ON CONFLICT (id) DO NOTHING
`, quoteIdentifier(schema)),
			account.id,
			account.username,
			strings.Title(account.username),
			account.email,
			now,
		); err != nil {
			t.Fatalf("seed account %s: %v", account.id, err)
		}

		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s.identity_devices (
	id, account_id, session_id, name, platform, status, public_key, push_token,
	created_at, last_seen_at, revoked_at, last_rotated_at
) VALUES ($1, $2, $3, $4, 'desktop', 'active', $5, '', $6, $6, NULL, NULL)
ON CONFLICT (id) DO NOTHING
`, quoteIdentifier(schema)),
			account.deviceID,
			account.id,
			account.sessionID,
			strings.Title(account.username)+" device",
			"seed-public-key",
			now,
		); err != nil {
			t.Fatalf("seed device %s: %v", account.deviceID, err)
		}

		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
INSERT INTO %s.identity_sessions (
	id, account_id, device_id, device_name, device_platform, ip_address, user_agent, status, current,
	created_at, last_seen_at, revoked_at
) VALUES ($1, $2, $3, $4, 'desktop', '', '', 'active', true, $5, $5, NULL)
ON CONFLICT (id) DO NOTHING
`, quoteIdentifier(schema)),
			account.sessionID,
			account.id,
			account.deviceID,
			strings.Title(account.username)+" device",
			now,
		); err != nil {
			t.Fatalf("seed session %s: %v", account.sessionID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed tx: %v", err)
	}
}

func openDockerPostgres(t *testing.T) *sql.DB {
	t.Helper()

	release := dockermutex.Acquire(t, "conversation-postgres")
	t.Cleanup(release)

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"docker",
		"run",
		"-d",
		"--rm",
		"--network",
		"host",
		"-e",
		"POSTGRES_PASSWORD=pass",
		"-e",
		"POSTGRES_DB=zvonilka",
		"postgres:16-alpine",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("docker postgres unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		t.Skip("docker returned an empty container id")
	}

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerID).Run()
	})

	db, err := sql.Open("pgx", "postgres://postgres:pass@127.0.0.1:5432/zvonilka?sslmode=disable")
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		if err := db.PingContext(ctx); err == nil {
			return db
		}
		time.Sleep(1 * time.Second)
	}

	_ = db.Close()
	t.Fatal("ping postgres: timed out")
	return nil
}

func repoMigrationsPath(t *testing.T, files ...string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	for _, file := range files {
		if _, err := os.Stat(filepath.Join(root, "deploy", "migrations", "postgres", file)); err != nil {
			t.Fatalf("missing migration %s: %v", file, err)
		}
	}

	return filepath.Join(root, "deploy", "migrations", "postgres")
}
