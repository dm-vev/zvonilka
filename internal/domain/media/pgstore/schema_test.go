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

	"github.com/dm-vev/zvonilka/internal/domain/media"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	"github.com/dm-vev/zvonilka/internal/testsupport/dockermutex"
)

func TestMediaSchema(t *testing.T) {
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
		"0007.sql",
		"0008.sql",
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	seedOwner(t, db, "tenant", "acc-owner", "owner", "owner@example.com")

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	t.Run("lifecycle", func(t *testing.T) {
		asset := media.MediaAsset{
			ID:              "media_1",
			OwnerAccountID:  "acc-owner",
			Kind:            media.MediaKindImage,
			Status:          media.MediaStatusReserved,
			StorageProvider: "object",
			Bucket:          "media-bucket",
			ObjectKey:       "media/acc-owner/media_1",
			FileName:        "photo.jpg",
			ContentType:     "image/jpeg",
			SizeBytes:       1024,
			SHA256Hex:       "abc123",
			Metadata:        map[string]string{"album": "vacation"},
			UploadExpiresAt: time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
			CreatedAt:       time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
			UpdatedAt:       time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
		}

		saved, err := store.SaveMediaAsset(context.Background(), asset)
		if err != nil {
			t.Fatalf("save media asset: %v", err)
		}
		if saved.ID != asset.ID {
			t.Fatalf("expected saved asset %s, got %s", asset.ID, saved.ID)
		}

		loaded, err := store.MediaAssetByID(context.Background(), asset.ID)
		if err != nil {
			t.Fatalf("load media asset: %v", err)
		}
		if loaded.ObjectKey != asset.ObjectKey {
			t.Fatalf("expected object key %s, got %s", asset.ObjectKey, loaded.ObjectKey)
		}

		assets, err := store.MediaAssetsByOwner(context.Background(), "acc-owner", 10)
		if err != nil {
			t.Fatalf("list media assets: %v", err)
		}
		if len(assets) != 1 {
			t.Fatalf("expected one asset, got %d", len(assets))
		}

		loadedByKey, err := store.MediaAssetByObjectKey(context.Background(), asset.ObjectKey)
		if err != nil {
			t.Fatalf("load by object key: %v", err)
		}
		if loadedByKey.ID != asset.ID {
			t.Fatalf("expected asset id %s, got %s", asset.ID, loadedByKey.ID)
		}
	})

	t.Run("constraints", func(t *testing.T) {
		cases := []struct {
			name       string
			args       []any
			constraint string
		}{
			{
				name: "invalid kind",
				args: mediaAssetInsertArgs(
					"media_invalid_kind",
					"alien",
					"reserved",
					time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					nil,
					nil,
					nil,
				),
				constraint: "media_assets_kind_check",
			},
			{
				name: "updated before created",
				args: mediaAssetInsertArgs(
					"media_invalid_updated_at",
					"image",
					"reserved",
					time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 11, 59, 59, 0, time.UTC),
					nil,
					nil,
					nil,
				),
				constraint: "media_assets_updated_after_created_check",
			},
			{
				name: "zero created at",
				args: mediaAssetInsertArgs(
					"media_invalid_created_at",
					"image",
					"reserved",
					time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
					time.Time{},
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					nil,
					nil,
					nil,
				),
				constraint: "media_assets_created_at_check",
			},
			{
				name: "zero updated at",
				args: mediaAssetInsertArgs(
					"media_invalid_updated_zero",
					"image",
					"reserved",
					time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Time{},
					nil,
					nil,
					nil,
				),
				constraint: "media_assets_updated_at_check",
			},
			{
				name: "zero upload expires at",
				args: mediaAssetInsertArgs(
					"media_invalid_upload_expires",
					"image",
					"reserved",
					time.Time{},
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					nil,
					nil,
					nil,
				),
				constraint: "media_assets_upload_expires_check",
			},
			{
				name: "ready without ready at",
				args: mediaAssetInsertArgs(
					"media_invalid_ready_state",
					"image",
					"ready",
					time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					nil,
					nil,
					nil,
				),
				constraint: "media_assets_ready_state_check",
			},
			{
				name: "zero ready at",
				args: mediaAssetInsertArgs(
					"media_invalid_ready_zero",
					"image",
					"deleted",
					time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Time{},
					time.Date(2026, time.March, 24, 12, 30, 0, 0, time.UTC),
					nil,
				),
				constraint: "media_assets_ready_at_check",
			},
			{
				name: "deleted without deleted at",
				args: mediaAssetInsertArgs(
					"media_invalid_deleted_state",
					"image",
					"deleted",
					time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					nil,
					nil,
					nil,
				),
				constraint: "media_assets_deleted_state_check",
			},
			{
				name: "zero deleted at",
				args: mediaAssetInsertArgs(
					"media_invalid_deleted_zero",
					"image",
					"deleted",
					time.Date(2026, time.March, 24, 12, 15, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					time.Date(2026, time.March, 24, 12, 0, 0, 0, time.UTC),
					nil,
					time.Time{},
					nil,
				),
				constraint: "media_assets_deleted_at_check",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := db.ExecContext(context.Background(), `
INSERT INTO tenant.media_assets (
	id, owner_account_id, kind, status, storage_provider, bucket, object_key, file_name,
	content_type, size_bytes, sha256_hex, width, height, duration_nanos, metadata,
	upload_expires_at, ready_at, deleted_at, created_at, updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8,
	$9, $10, $11, $12, $13, $14, $15,
	$16, $17, $18, $19, $20
)`, tc.args...)
				if err == nil {
					t.Fatalf("expected %s to fail", tc.name)
				}

				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) {
					t.Fatalf("expected postgres error for %s, got %v", tc.name, err)
				}
				if pgErr.ConstraintName != tc.constraint {
					t.Fatalf("expected constraint %s, got %s (%v)", tc.constraint, pgErr.ConstraintName, err)
				}
			})
		}
	})
}

func mediaAssetInsertArgs(
	id string,
	kind string,
	status string,
	uploadExpiresAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
	readyAt any,
	deletedAt any,
	metadata any,
) []any {
	if metadata == nil {
		metadata = `{"album":"vacation"}`
	}

	return []any{
		id,
		"acc-owner",
		kind,
		status,
		"object",
		"media-bucket",
		"media/acc-owner/" + id,
		"photo.jpg",
		"image/jpeg",
		1024,
		"abc123",
		0,
		0,
		0,
		metadata,
		uploadExpiresAt,
		readyAt,
		deletedAt,
		createdAt,
		updatedAt,
	}
}

func seedOwner(t *testing.T, db *sql.DB, schema string, accountID string, username string, email string) {
	t.Helper()

	_, err := db.ExecContext(context.Background(), fmt.Sprintf(`
INSERT INTO %s.identity_accounts (
	id, kind, username, display_name, bio, email, phone, roles, status, bot_token_hash,
	created_by, created_at, updated_at, disabled_at, last_auth_at, custom_badge_emoji
) VALUES ($1, 'user', $2, $2, '', $3, '', '[]', 'active', '', 'seed', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, NULL, NULL, '')
ON CONFLICT (id) DO NOTHING
`, qualifiedName(schema, "identity_accounts")),
		accountID,
		username,
		email,
	)
	if err != nil {
		t.Fatalf("seed owner account: %v", err)
	}
}

func openDockerPostgres(t *testing.T) *sql.DB {
	t.Helper()

	unlock := dockermutex.Acquire(t, "postgres-integration")
	t.Cleanup(unlock)

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

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 4*time.Minute)
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

func repoMigrationsPath(t *testing.T, files ...string) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}

	base := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "deploy", "migrations", "postgres")
	if len(files) == 0 {
		return base
	}

	dir := t.TempDir()
	for _, name := range files {
		src := filepath.Join(base, name)
		raw, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read migration %s: %v", src, err)
		}
		dst := filepath.Join(dir, name)
		if err := os.WriteFile(dst, raw, 0o600); err != nil {
			t.Fatalf("write migration %s: %v", dst, err)
		}
	}

	return dir
}
