package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/dm-vev/zvonilka/internal/domain/notification"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	"github.com/dm-vev/zvonilka/internal/testsupport/dockermutex"
)

func TestNotificationSchemaConstraints(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t)
	require.NoError(t, platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"))

	seedNotificationReferences(t, db, "tenant")

	store, err := New(db, "tenant")
	require.NoError(t, err)

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	t.Run("rejects blank preference account id", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_preferences (
	account_id, enabled, direct_enabled, group_enabled, channel_enabled, mention_enabled, reply_enabled,
	quiet_hours_enabled, quiet_hours_start_minute, quiet_hours_end_minute, quiet_hours_timezone, muted_until,
	updated_at
) VALUES (
	$1, TRUE, TRUE, TRUE, TRUE, TRUE, TRUE,
	FALSE, 0, 0, '', NULL,
	$2
)`,
			"",
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank push token id", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_push_tokens (
	id, account_id, device_id, provider, token, platform, enabled, created_at, updated_at, revoked_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)`,
			"",
			"acc-1",
			"dev-1",
			"apns",
			"token-0",
			"web",
			true,
			now,
			now,
			nil,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank conversation override conversation id", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_conversation_overrides (
	conversation_id, account_id, muted, mentions_only, muted_until, updated_at
) VALUES (
	$1, $2, FALSE, FALSE, NULL, $3
)`,
			"",
			"acc-1",
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank push token provider", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_push_tokens (
	id, account_id, device_id, provider, token, platform, enabled, created_at, updated_at, revoked_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)`,
			"push-1",
			"acc-1",
			"dev-1",
			"",
			"token-1",
			"web",
			true,
			now,
			now,
			nil,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects mixed-case push token provider", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_push_tokens (
	id, account_id, device_id, provider, token, platform, enabled, created_at, updated_at, revoked_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)`,
			"push-2",
			"acc-1",
			"dev-1",
			"APNS",
			"token-2",
			"web",
			true,
			now,
			now,
			nil,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank worker cursor name", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_worker_cursors (
	name, last_sequence, updated_at
) VALUES (
	$1, $2, $3
)`,
			"",
			0,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects negative worker cursor sequence", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_worker_cursors (
	name, last_sequence, updated_at
) VALUES (
	$1, $2, $3
)`,
			"conversation_notifications",
			-1,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects mixed-case worker cursor name", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_worker_cursors (
	name, last_sequence, updated_at
) VALUES (
	$1, $2, $3
)`,
			"Conversation_Notifications",
			0,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank delivery id", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_deliveries (
	id, dedup_key, event_id, conversation_id, message_id, account_id, device_id,
	push_token_id, kind, reason, mode, state, priority, attempts, next_attempt_at,
	last_attempt_at, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19
)`,
			"",
			"evt-1:conv-1:msg-1:acc-1::group:group",
			"evt-1",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			"group",
			"group",
			"in_app",
			"queued",
			0,
			0,
			now,
			nil,
			"",
			now,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank delivery dedup key", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_deliveries (
	id, dedup_key, event_id, conversation_id, message_id, account_id, device_id,
	push_token_id, kind, reason, mode, state, priority, attempts, next_attempt_at,
	last_attempt_at, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19
)`,
			"del-blank-dedup",
			"",
			"evt-1",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			"group",
			"group",
			"in_app",
			"queued",
			0,
			0,
			now,
			nil,
			"",
			now,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank delivery event id", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_deliveries (
	id, dedup_key, event_id, conversation_id, message_id, account_id, device_id,
	push_token_id, kind, reason, mode, state, priority, attempts, next_attempt_at,
	last_attempt_at, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19
)`,
			"del-blank-event",
			":conv-1:msg-1:acc-1::group:group",
			"",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			"group",
			"group",
			"in_app",
			"queued",
			0,
			0,
			now,
			nil,
			"",
			now,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank delivery account id", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_deliveries (
	id, dedup_key, event_id, conversation_id, message_id, account_id, device_id,
	push_token_id, kind, reason, mode, state, priority, attempts, next_attempt_at,
	last_attempt_at, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19
)`,
			"del-blank-account",
			"evt-1:conv-1:msg-1:::group:group",
			"evt-1",
			"conv-1",
			"msg-1",
			"",
			"",
			"",
			"group",
			"group",
			"in_app",
			"queued",
			0,
			0,
			now,
			nil,
			"",
			now,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank delivery conversation id", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_deliveries (
	id, dedup_key, event_id, conversation_id, message_id, account_id, device_id,
	push_token_id, kind, reason, mode, state, priority, attempts, next_attempt_at,
	last_attempt_at, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19
)`,
			"del-blank-conversation",
			"evt-1::msg-1:acc-1::group:group",
			"evt-1",
			"",
			"msg-1",
			"acc-1",
			"",
			"",
			"group",
			"group",
			"in_app",
			"queued",
			0,
			0,
			now,
			nil,
			"",
			now,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank delivery message id", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_deliveries (
	id, dedup_key, event_id, conversation_id, message_id, account_id, device_id,
	push_token_id, kind, reason, mode, state, priority, attempts, next_attempt_at,
	last_attempt_at, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19
)`,
			"del-blank-message",
			"evt-1:conv-1::acc-1::group:group",
			"evt-1",
			"conv-1",
			"",
			"acc-1",
			"",
			"",
			"group",
			"group",
			"in_app",
			"queued",
			0,
			0,
			now,
			nil,
			"",
			now,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects blank delivery reason", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_deliveries (
	id, dedup_key, event_id, conversation_id, message_id, account_id, device_id,
	push_token_id, kind, reason, mode, state, priority, attempts, next_attempt_at,
	last_attempt_at, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19
)`,
			"del-blank-reason",
			"evt-1:conv-1:msg-1:acc-1::group:",
			"evt-1",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			"group",
			"",
			"in_app",
			"queued",
			0,
			0,
			now,
			nil,
			"",
			now,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects push mode without device targets", func(t *testing.T) {
		_, err := db.ExecContext(ctx, `
INSERT INTO tenant.notification_deliveries (
	id, dedup_key, event_id, conversation_id, message_id, account_id, device_id,
	push_token_id, kind, reason, mode, state, priority, attempts, next_attempt_at,
	last_attempt_at, last_error, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19
)`,
			"del-push",
			"evt-1:conv-1:msg-1:acc-1::group:group:push",
			"evt-1",
			"conv-1",
			"msg-1",
			"acc-1",
			"",
			"",
			"group",
			"group",
			"push",
			"queued",
			0,
			0,
			now,
			nil,
			"",
			now,
			now,
		)
		requirePgCode(t, err, "23514")
	})

	t.Run("serializes concurrent retry attempts", func(t *testing.T) {
		saved, err := store.SaveDelivery(ctx, notification.Delivery{
			ID:             "del-race",
			DedupKey:       "evt-1:conv-1:msg-1:acc-1::group:group:race",
			EventID:        "evt-1",
			ConversationID: "conv-1",
			MessageID:      "msg-1",
			AccountID:      "acc-1",
			Kind:           notification.NotificationKindGroup,
			Reason:         "group",
			Mode:           notification.DeliveryModeInApp,
			State:          notification.DeliveryStateQueued,
			Priority:       10,
			NextAttemptAt:  now,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		require.NoError(t, err)

		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, retryErr := store.RetryDelivery(ctx, notification.RetryDeliveryParams{
					DeliveryID: saved.ID,
					LastError:  "push failed",
					RetryAt:    now.Add(time.Second),
				})
				errs <- retryErr
			}()
		}

		close(start)
		wg.Wait()
		close(errs)

		for err := range errs {
			require.NoError(t, err)
		}

		reloaded, err := store.DeliveryByID(ctx, saved.ID)
		require.NoError(t, err)
		require.Equal(t, 2, reloaded.Attempts)
	})
}

func seedNotificationReferences(t *testing.T, db *sql.DB, schema string) {
	t.Helper()

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin seed tx: %v", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Date(2026, time.March, 25, 11, 59, 0, 0, time.UTC)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (
	id, kind, username, display_name, bio, email, phone, roles, status, bot_token_hash,
	created_by, created_at, updated_at, disabled_at, last_auth_at, custom_badge_emoji
) VALUES ($1, 'user', $2, $3, '', $4, '', '[]', 'active', '', 'seed', $5, $5, NULL, NULL, '')
ON CONFLICT (id) DO NOTHING
`, qualifiedName(schema, "identity_accounts")),
		"acc-1",
		"alice",
		"Alice",
		"alice@example.com",
		now,
	); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (
	id, account_id, session_id, name, platform, status, public_key, push_token,
	created_at, last_seen_at, revoked_at, last_rotated_at
) VALUES ($1, $2, $3, $4, 'web', 'active', $5, '', $6, $6, NULL, NULL)
ON CONFLICT (id) DO NOTHING
`, qualifiedName(schema, "identity_devices")),
		"dev-1",
		"acc-1",
		"sess-1",
		"Alice web",
		"seed-public-key",
		now,
	); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (
	id, account_id, device_id, device_name, device_platform, ip_address, user_agent, status, current,
	created_at, last_seen_at, revoked_at
) VALUES ($1, $2, $3, $4, 'web', '', '', 'active', true, $5, $5, NULL)
ON CONFLICT (id) DO NOTHING
`, qualifiedName(schema, "identity_sessions")),
		"sess-1",
		"acc-1",
		"dev-1",
		"Alice web",
		now,
	); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (
	id, kind, title, description, avatar_media_id, owner_account_id,
	only_admins_can_write, only_admins_can_add_members, allow_reactions, allow_forwards,
	allow_threads, require_join_approval, pinned_messages_only_admins, slow_mode_interval_nanos,
	archived, muted, pinned, hidden, last_sequence, created_at, updated_at, last_message_at
) VALUES (
	$1, 'group', $2, '', '', $3,
	FALSE, FALSE, TRUE, TRUE,
	TRUE, FALSE, FALSE, 0,
	FALSE, FALSE, FALSE, FALSE, 0, $4, $4, NULL
)
ON CONFLICT (id) DO NOTHING
`, qualifiedName(schema, "conversation_conversations")),
		"conv-1",
		"Group",
		"acc-1",
		now,
	); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
	INSERT INTO %s (
	conversation_id, id, title, created_by_account_id, is_general, archived, pinned, closed,
	last_sequence, message_count, created_at, updated_at, last_message_at, archived_at, closed_at
) VALUES (
	$1, '', 'General', $2, TRUE, FALSE, FALSE, FALSE,
	0, 0, $3, $3, NULL, NULL, NULL
)
ON CONFLICT (conversation_id, id) DO NOTHING
`, qualifiedName(schema, "conversation_topics")),
		"conv-1",
		"acc-1",
		now,
	); err != nil {
		t.Fatalf("seed general topic: %v", err)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
	INSERT INTO %s (
	id, conversation_id, sender_account_id, sender_device_id, kind, status,
	payload_key_id, payload_algorithm, payload_nonce, payload_ciphertext, payload_aad,
	created_at, updated_at
) VALUES ($1, $2, $3, $4, 'text', 'sent', $5, 'xchacha20poly1305', decode('01', 'hex'),
	decode('02', 'hex'), decode('03', 'hex'), $6, $6)
ON CONFLICT (id) DO NOTHING
`, qualifiedName(schema, "conversation_messages")),
		"msg-1",
		"conv-1",
		"acc-1",
		"dev-1",
		"key-1",
		now,
	); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed tx: %v", err)
	}
}

func requirePgCode(t *testing.T, err error, code string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected postgres error %s", code)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected postgres error %s, got %T: %v", code, err, err)
	}
	if pgErr.Code != code {
		t.Fatalf("expected postgres error %s, got %s: %v", code, pgErr.Code, err)
	}
}

func openDockerPostgres(t *testing.T) *sql.DB {
	t.Helper()

	unlock := dockermutex.Acquire(t, "notification-postgres")
	t.Cleanup(unlock)

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	cfg := postgresDockerConfigForGOOS(runtime.GOOS)
	cmd := exec.CommandContext(
		ctx,
		"docker",
		cfg.runArgs...,
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

	dsn := cfg.dsn
	if cfg.needsPortMapping {
		portOut, err := exec.CommandContext(ctx, "docker", "port", containerID, "5432/tcp").CombinedOutput()
		if err != nil {
			t.Skipf("lookup postgres port: %v: %s", err, strings.TrimSpace(string(portOut)))
		}
		hostPort := strings.TrimSpace(string(portOut))
		if hostPort == "" {
			t.Skip("docker did not report a mapped port")
		}
		hostPort = hostPort[strings.LastIndex(hostPort, ":")+1:]
		dsn = fmt.Sprintf(cfg.dsn, hostPort)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres database: %v", err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer pingCancel()
	for pingCtx.Err() == nil {
		if err := db.PingContext(pingCtx); err == nil {
			return db
		}
		time.Sleep(1 * time.Second)
	}

	_ = db.Close()
	t.Skip("postgres container did not become ready")
	return nil
}

type postgresDockerConfig struct {
	runArgs          []string
	dsn              string
	needsPortMapping bool
}

func postgresDockerConfigForGOOS(goos string) postgresDockerConfig {
	if goos == "linux" {
		return postgresDockerConfig{
			runArgs: []string{
				"run",
				"-d",
				"--rm",
				"--network",
				"host",
				"-e",
				"POSTGRES_PASSWORD=pass",
				"-e",
				"POSTGRES_DB=test",
				"postgres:16-alpine",
			},
			dsn: "postgres://postgres:pass@127.0.0.1:5432/test?sslmode=disable",
		}
	}

	return postgresDockerConfig{
		runArgs: []string{
			"run",
			"-d",
			"--rm",
			"-e",
			"POSTGRES_PASSWORD=pass",
			"-e",
			"POSTGRES_DB=test",
			"-p",
			"127.0.0.1::5432",
			"postgres:16-alpine",
		},
		dsn:              "postgres://postgres:pass@127.0.0.1:%s/test?sslmode=disable",
		needsPortMapping: true,
	}
}

func repoMigrationsPath(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}

	return filepath.Join(
		filepath.Dir(file),
		"..",
		"..",
		"..",
		"..",
		"deploy",
		"migrations",
		"postgres",
	)
}
