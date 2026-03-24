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
		OwnerAccountID:  "id-owner",
		Kind:            conversation.ConversationKindDirect,
		Title:           "Direct",
		MemberAccountIDs: []string{"id-peer"},
		CreatedAt:       time.Date(2026, time.March, 24, 11, 0, 0, 0, time.UTC),
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
		ConversationID:        created.ID,
		AccountID:             "id-peer",
		DeviceID:              "dev-peer",
		MessageID:             message.ID,
		DeliveredThroughSequence: event.Sequence,
		CreatedAt:             time.Date(2026, time.March, 24, 11, 2, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("record delivery: %v", err)
	}

	readState, _, err := svc.MarkRead(context.Background(), conversation.MarkReadParams{
		ConversationID:   created.ID,
		AccountID:        "id-peer",
		DeviceID:         "dev-peer",
		ReadThroughSequence: event.Sequence,
		CreatedAt:        time.Date(2026, time.March, 24, 11, 3, 0, 0, time.UTC),
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

	loadedMessage, err := store.MessageByID(context.Background(), created.ID, message.ID)
	if err != nil {
		t.Fatalf("load message: %v", err)
	}
	if len(loadedMessage.Attachments) != 1 {
		t.Fatalf("expected loaded attachment, got %d", len(loadedMessage.Attachments))
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
		id       string
		username string
		email    string
		deviceID string
		sessionID string
	}{
		{ownerID, ownerUsername, ownerEmail, ownerDeviceID, ownerSessionID},
	}
	if len(peer) >= 5 {
		accounts = append(accounts, struct {
			id       string
			username string
			email    string
			deviceID string
			sessionID string
		}{
			id:       peer[0],
			username: peer[1],
			email:    peer[2],
			deviceID: peer[3],
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

	if err := runDockerCompose("up", "-d", "postgres"); err != nil {
		t.Skipf("docker postgres unavailable: %v", err)
	}

	dsn := "postgres://zvonilka:zvonilka@localhost:5432/zvonilka?sslmode=disable"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	return db
}

func runDockerCompose(args ...string) error {
	commandArgs := append([]string{"compose", "-f", "deploy/local/docker-compose.yml"}, args...)
	cmd := exec.Command("docker", commandArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
