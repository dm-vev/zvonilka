package pgstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dm-vev/zvonilka/internal/domain/conversation"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

func TestEncryptedPayloadRejectsInvalidMessage(t *testing.T) {
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
		return time.Date(2026, time.March, 24, 18, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID:   "id-owner",
		Kind:             conversation.ConversationKindGroup,
		Title:            "Group",
		MemberAccountIDs: []string{"id-peer"},
		CreatedAt:        time.Date(2026, time.March, 24, 18, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	_, err = store.SaveMessage(context.Background(), conversation.Message{
		ID:              "msg-invalid",
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Kind:            conversation.MessageKindText,
		Status:          conversation.MessageStatusSent,
		Payload:         conversation.EncryptedPayload{},
		CreatedAt:       time.Date(2026, time.March, 24, 18, 1, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, time.March, 24, 18, 1, 0, 0, time.UTC),
	})
	if !errors.Is(err, conversation.ErrInvalidInput) {
		t.Fatalf("expected invalid payload to fail, got %v", err)
	}
}

func TestEncryptedPayloadStripsPlaintextHints(t *testing.T) {
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
		return time.Date(2026, time.March, 24, 19, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, _, err := svc.CreateConversation(context.Background(), conversation.CreateConversationParams{
		OwnerAccountID:   "id-owner",
		Kind:             conversation.ConversationKindGroup,
		Title:            "Group",
		MemberAccountIDs: []string{"id-peer"},
		CreatedAt:        time.Date(2026, time.March, 24, 19, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	rootMessage, _, err := svc.SendMessage(context.Background(), conversation.SendMessageParams{
		ConversationID:  created.ID,
		SenderAccountID: "id-owner",
		SenderDeviceID:  "dev-owner",
		Draft: conversation.MessageDraft{
			Kind: conversation.MessageKindText,
			Payload: conversation.EncryptedPayload{
				Ciphertext: []byte("root-body"),
			},
		},
		CreatedAt: time.Date(2026, time.March, 24, 19, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("send root message: %v", err)
	}

	saved, err := store.SaveMessage(context.Background(), conversation.Message{
		ID:              "msg-pgstore",
		ConversationID:  created.ID,
		SenderAccountID: "id-peer",
		SenderDeviceID:  "dev-peer",
		Kind:            conversation.MessageKindText,
		Status:          conversation.MessageStatusSent,
		Payload: conversation.EncryptedPayload{
			Ciphertext: []byte("reply-body"),
		},
		ReplyTo: conversation.MessageReference{
			ConversationID:  created.ID,
			MessageID:       rootMessage.ID,
			SenderAccountID: "id-owner",
			MessageKind:     conversation.MessageKindText,
			Snippet:         "plaintext quote",
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
		CreatedAt: time.Date(2026, time.March, 24, 19, 2, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.March, 24, 19, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("save message: %v", err)
	}
	if saved.ReplyTo.Snippet != "" {
		t.Fatalf("expected persisted reply snippet to be stripped, got %q", saved.ReplyTo.Snippet)
	}
	if len(saved.Attachments) != 1 {
		t.Fatalf("expected one attachment, got %d", len(saved.Attachments))
	}
	if saved.Attachments[0].Caption != "" {
		t.Fatalf("expected persisted attachment caption to be stripped, got %q", saved.Attachments[0].Caption)
	}

	loaded, err := store.MessageByID(context.Background(), created.ID, saved.ID)
	if err != nil {
		t.Fatalf("load message: %v", err)
	}
	if loaded.ReplyTo.Snippet != "" {
		t.Fatalf("expected loaded reply snippet to be stripped, got %q", loaded.ReplyTo.Snippet)
	}
	if len(loaded.Attachments) != 1 {
		t.Fatalf("expected loaded attachment, got %d", len(loaded.Attachments))
	}
	if loaded.Attachments[0].Caption != "" {
		t.Fatalf("expected loaded attachment caption to be stripped, got %q", loaded.Attachments[0].Caption)
	}
}
