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
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dm-vev/zvonilka/internal/domain/identity"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

func TestIdentitySchemaHardening(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, time.March, 23, 23, 45, 0, 0, time.UTC)

	account, err := store.SaveAccount(ctx, identity.Account{
		ID:          "acc-1",
		Kind:        identity.AccountKindUser,
		Username:    "alice",
		DisplayName: "Alice",
		Status:      identity.AccountStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("save valid account: %v", err)
	}

	t.Run("rejects invalid account kind", func(t *testing.T) {
		_, err := store.SaveAccount(ctx, identity.Account{
			ID:          "acc-kind",
			Kind:        identity.AccountKind("alien"),
			Username:    "alien",
			DisplayName: "Alien",
			Status:      identity.AccountStatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects bot without token hash", func(t *testing.T) {
		_, err := store.SaveAccount(ctx, identity.Account{
			ID:          "acc-bot",
			Kind:        identity.AccountKindBot,
			Username:    "bot",
			DisplayName: "Bot",
			Status:      identity.AccountStatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects invalid account status", func(t *testing.T) {
		_, err := store.SaveAccount(ctx, identity.Account{
			ID:          "acc-status",
			Kind:        identity.AccountKindUser,
			Username:    "status",
			DisplayName: "Status",
			Status:      identity.AccountStatus("ghost"),
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects invalid join request status", func(t *testing.T) {
		_, err := store.SaveJoinRequest(ctx, identity.JoinRequest{
			ID:          "join-1",
			Username:    "alice-join",
			DisplayName: "Alice Join",
			Status:      identity.JoinRequestStatus("bogus"),
			RequestedAt: now,
			ExpiresAt:   now.Add(time.Hour),
		})
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects invalid login challenge delivery channel", func(t *testing.T) {
		_, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{
			ID:              "chal-1",
			AccountID:       account.ID,
			AccountKind:     identity.AccountKindUser,
			CodeHash:        "hash",
			DeliveryChannel: identity.LoginDeliveryChannel("fax"),
			ExpiresAt:       now.Add(time.Hour),
			CreatedAt:       now,
		})
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects mismatched login challenge used pair", func(t *testing.T) {
		_, err := store.SaveLoginChallenge(ctx, identity.LoginChallenge{
			ID:              "chal-2",
			AccountID:       account.ID,
			AccountKind:     identity.AccountKindUser,
			CodeHash:        "hash",
			DeliveryChannel: identity.LoginDeliveryChannelEmail,
			ExpiresAt:       now.Add(time.Hour),
			CreatedAt:       now,
			Used:            true,
		})
		requirePgCode(t, err, "23514")
	})

	const sessionID = "sess-1"
	const primaryDeviceID = "dev-1"

	t.Run("allows deferred device then session insert", func(t *testing.T) {
		err := store.WithinTx(ctx, func(tx identity.Store) error {
			if _, err := tx.SaveDevice(ctx, identity.Device{
				ID:         primaryDeviceID,
				AccountID:  account.ID,
				SessionID:  sessionID,
				Name:       "Alice laptop",
				Platform:   identity.DevicePlatformWeb,
				Status:     identity.DeviceStatusActive,
				PublicKey:  "pk-1",
				CreatedAt:  now,
				LastSeenAt: now,
			}); err != nil {
				return err
			}
			if _, err := tx.SaveSession(ctx, identity.Session{
				ID:             sessionID,
				AccountID:      account.ID,
				DeviceID:       primaryDeviceID,
				DeviceName:     "Alice laptop",
				DevicePlatform: identity.DevicePlatformWeb,
				Status:         identity.SessionStatusActive,
				Current:        true,
				CreatedAt:      now,
				LastSeenAt:     now,
			}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("save deferred device/session pair: %v", err)
		}

		device, err := store.DeviceByID(ctx, primaryDeviceID)
		if err != nil {
			t.Fatalf("load deferred device: %v", err)
		}
		if device.SessionID != sessionID {
			t.Fatalf("expected session %q, got %q", sessionID, device.SessionID)
		}
	})

	t.Run("rejects invalid device session reference", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-orphan",
			AccountID:  account.ID,
			SessionID:  "sess-missing",
			Name:       "Orphan",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-orphan",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		if !errors.Is(err, identity.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("rejects invalid device platform", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-platform",
			AccountID:  account.ID,
			SessionID:  sessionID,
			Name:       "Tablet",
			Platform:   identity.DevicePlatform("tvos"),
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-platform",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects invalid device status", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-status",
			AccountID:  account.ID,
			SessionID:  sessionID,
			Name:       "Tablet",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatus("ghost"),
			PublicKey:  "pk-status",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects invalid session status", func(t *testing.T) {
		session, err := store.SessionByID(ctx, sessionID)
		if err != nil {
			t.Fatalf("load valid session: %v", err)
		}
		session.Status = identity.SessionStatus("ghost")
		_, err = store.UpdateSession(ctx, session)
		requirePgCode(t, err, "23514")
	})

	t.Run("rejects session deletion when attached devices remain", func(t *testing.T) {
		_, err := store.SaveDevice(ctx, identity.Device{
			ID:         "dev-extra",
			AccountID:  account.ID,
			SessionID:  sessionID,
			Name:       "Alice tablet",
			Platform:   identity.DevicePlatformWeb,
			Status:     identity.DeviceStatusActive,
			PublicKey:  "pk-extra",
			CreatedAt:  now,
			LastSeenAt: now,
		})
		if err != nil {
			t.Fatalf("save extra device: %v", err)
		}

		if err := store.DeleteSession(ctx, sessionID); !errors.Is(err, identity.ErrConflict) {
			t.Fatalf("expected conflict, got %v", err)
		}
	})

	t.Run("exposes pending expiry index", func(t *testing.T) {
		var indexName sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1)`, qualifiedName("tenant", "identity_join_requests_pending_expires_at_idx")).Scan(&indexName); err != nil {
			t.Fatalf("load expiry index: %v", err)
		}
		if !indexName.Valid || indexName.String == "" {
			t.Fatal("expected pending expiry index to exist")
		}
	})
}

func openDockerPostgres(t *testing.T) *sql.DB {
	t.Helper()

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
		"-e",
		"POSTGRES_PASSWORD=pass",
		"-e",
		"POSTGRES_DB=test",
		"-p",
		"127.0.0.1::5432",
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

	portOut, err := exec.CommandContext(ctx, "docker", "port", containerID, "5432/tcp").CombinedOutput()
	if err != nil {
		t.Skipf("lookup postgres port: %v: %s", err, strings.TrimSpace(string(portOut)))
	}
	hostPort := strings.TrimSpace(string(portOut))
	if hostPort == "" {
		t.Skip("docker did not report a mapped port")
	}
	hostPort = hostPort[strings.LastIndex(hostPort, ":")+1:]

	dsn := fmt.Sprintf("postgres://postgres:pass@127.0.0.1:%s/test?sslmode=disable", hostPort)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres database: %v", err)
	}

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 90*time.Second)
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

func repoMigrationsPath(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}

	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "deploy", "migrations", "postgres")
}

func requirePgCode(t *testing.T, err error, code string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected postgres error code %s, got nil", code)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected postgres error code %s, got %v", code, err)
	}
	if pgErr.Code != code {
		t.Fatalf("expected postgres error code %s, got %s (%v)", code, pgErr.Code, err)
	}
}
