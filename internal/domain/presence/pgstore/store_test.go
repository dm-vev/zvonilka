package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	identitypgstore "github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
	"github.com/dm-vev/zvonilka/internal/domain/presence"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	"github.com/dm-vev/zvonilka/internal/testsupport/dockermutex"
)

func TestPresenceSchemaLifecycle(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t,
		"0001.sql",
		"0002_identity_hardening.sql",
		"0003_identity_account_boundaries.sql",
		"0004_identity_session_device_deferrable.sql",
		"0013.sql",
	)
	if err := applyMigrations(db, migrationsPath); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedIdentity(t, db, "tenant",
		"id-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
	)

	identityStore, err := identitypgstore.New(db, "tenant")
	if err != nil {
		t.Fatalf("new identity store: %v", err)
	}
	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new presence store: %v", err)
	}
	svc, err := presence.NewService(store, identityStore, presence.WithNow(func() time.Time {
		return time.Date(2026, time.March, 24, 18, 0, 0, 0, time.UTC)
	}))
	if err != nil {
		t.Fatalf("new presence service: %v", err)
	}

	snapshot, err := svc.GetPresence(context.Background(), presence.GetParams{AccountID: "id-owner"})
	if err != nil {
		t.Fatalf("get presence: %v", err)
	}
	if snapshot.State != presence.PresenceStateOnline {
		t.Fatalf("expected online state, got %s", snapshot.State)
	}
	if snapshot.LastSeenHidden {
		t.Fatal("expected last seen to be visible")
	}

	saved, err := svc.SetPresence(context.Background(), presence.SetParams{
		AccountID:       "id-owner",
		State:           presence.PresenceStateBusy,
		CustomStatus:    "In a call",
		HideLastSeenFor: 15 * time.Minute,
		RecordedAt:      time.Date(2026, time.March, 24, 18, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("set presence: %v", err)
	}
	if saved.State != presence.PresenceStateBusy {
		t.Fatalf("expected busy state, got %s", saved.State)
	}
	if saved.HiddenUntil.IsZero() {
		t.Fatal("expected hidden until timestamp")
	}

	other, err := svc.GetPresence(context.Background(), presence.GetParams{
		AccountID:       "id-owner",
		ViewerAccountID: "viewer",
	})
	if err != nil {
		t.Fatalf("get presence for other viewer: %v", err)
	}
	if other.State != presence.PresenceStateBusy {
		t.Fatalf("expected busy state, got %s", other.State)
	}
	if !other.LastSeenHidden {
		t.Fatal("expected last seen to be hidden for other viewers")
	}

	self, err := svc.GetPresence(context.Background(), presence.GetParams{
		AccountID:       "id-owner",
		ViewerAccountID: "id-owner",
	})
	if err != nil {
		t.Fatalf("get presence for self: %v", err)
	}
	if self.LastSeenHidden {
		t.Fatal("expected self to see last seen")
	}
	if self.LastSeenAt.IsZero() {
		t.Fatal("expected self to see last seen timestamp")
	}
}

func TestPresenceSchemaConstraints(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t,
		"0001.sql",
		"0002_identity_hardening.sql",
		"0003_identity_account_boundaries.sql",
		"0004_identity_session_device_deferrable.sql",
		"0013.sql",
	)
	if err := applyMigrations(db, migrationsPath); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedIdentity(t, db, "tenant",
		"id-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
	)

	_, err := db.ExecContext(context.Background(), `
INSERT INTO tenant.user_presence (
	account_id, state, custom_status, hidden_until, updated_at
) VALUES (
	$1, $2, $3, $4, $5
)`,
		"id-owner",
		"party",
		"",
		nil,
		time.Date(2026, time.March, 24, 18, 10, 0, 0, time.UTC),
	)
	if err == nil {
		t.Fatal("expected invalid presence state to fail")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected pg error, got %v", err)
	}
	if pgErr.Code != "23514" {
		t.Fatalf("expected check violation, got %s", pgErr.Code)
	}
	if pgErr.ConstraintName != "user_presence_state_check" {
		t.Fatalf("expected presence state constraint, got %s", pgErr.ConstraintName)
	}
}

func TestSavePresenceRejectsMissingAccount(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t,
		"0001.sql",
		"0002_identity_hardening.sql",
		"0003_identity_account_boundaries.sql",
		"0004_identity_session_device_deferrable.sql",
		"0013.sql",
	)
	if err := applyMigrations(db, migrationsPath); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new presence store: %v", err)
	}

	_, err = store.SavePresence(context.Background(), presence.Presence{
		AccountID: "missing",
		State:     presence.PresenceStateOnline,
		UpdatedAt: time.Date(2026, time.March, 24, 18, 20, 0, 0, time.UTC),
	})
	if !errors.Is(err, presence.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestSavePresenceRejectsZeroUpdatedAt(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t,
		"0001.sql",
		"0002_identity_hardening.sql",
		"0003_identity_account_boundaries.sql",
		"0004_identity_session_device_deferrable.sql",
		"0013.sql",
	)
	if err := applyMigrations(db, migrationsPath); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedIdentity(t, db, "tenant",
		"id-owner", "owner", "owner@example.com", "dev-owner", "sess-owner",
	)

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new presence store: %v", err)
	}

	_, err = store.SavePresence(context.Background(), presence.Presence{
		AccountID: "id-owner",
		State:     presence.PresenceStateOnline,
	})
	if !errors.Is(err, presence.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func seedIdentity(t *testing.T, db *sql.DB, schema string, ownerID, ownerUsername, ownerEmail, ownerDeviceID, ownerSessionID string) {
	t.Helper()

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin seed tx: %v", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Date(2026, time.March, 24, 18, 0, 0, 0, time.UTC)
	queries := []struct {
		query string
		args  []any
	}{
		{
			query: fmt.Sprintf(`
INSERT INTO %s.identity_accounts (
	id, kind, username, display_name, bio, email, phone, roles, status, bot_token_hash,
	created_by, created_at, updated_at, disabled_at, last_auth_at, custom_badge_emoji
) VALUES (
	$1, 'user', $2, $3, '', $4, '', '[]', 'active', '',
	'', $5, $5, NULL, $5, ''
)`, schema),
			args: []any{ownerID, ownerUsername, strings.ToUpper(ownerUsername[:1]) + ownerUsername[1:], ownerEmail, now},
		},
		{
			query: fmt.Sprintf(`
INSERT INTO %s.identity_devices (
	id, account_id, session_id, name, platform, status, public_key, push_token, created_at, last_seen_at, revoked_at, last_rotated_at
) VALUES (
	$1, $2, $3, 'Device', 'web', 'active', '', '', $4, $4, NULL, NULL
)`, schema),
			args: []any{ownerDeviceID, ownerID, ownerSessionID, now.Add(-time.Minute)},
		},
		{
			query: fmt.Sprintf(`
INSERT INTO %s.identity_sessions (
	id, account_id, device_id, device_name, device_platform, ip_address, user_agent, status, current, created_at, last_seen_at, revoked_at
) VALUES (
	$1, $2, $3, 'Device', 'web', '', '', 'active', true, $4, $4, NULL
)`, schema),
			args: []any{ownerSessionID, ownerID, ownerDeviceID, now.Add(-time.Minute)},
		},
	}

	for _, item := range queries {
		if _, err := tx.ExecContext(ctx, item.query, item.args...); err != nil {
			t.Fatalf("seed identity: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed tx: %v", err)
	}
}

func openDockerPostgres(t *testing.T) *sql.DB {
	t.Helper()

	release := dockermutex.Acquire(t, "postgres-integration")
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

func applyMigrations(db *sql.DB, migrationsPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	return platformpostgres.ApplyMigrations(ctx, db, migrationsPath, "tenant")
}
