package postgres

import (
	"context"
	"database/sql"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dm-vev/zvonilka/internal/testsupport/dockermutex"
)

func TestApplyMigrationsIsIdempotent(t *testing.T) {
	db := openDockerPostgres(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	migrationsPath := repoMigrationsPath(t)
	ctx := context.Background()

	if err := ApplyMigrations(ctx, db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations first time: %v", err)
	}
	if err := ApplyMigrations(ctx, db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations second time: %v", err)
	}

	expectedCount, err := filepath.Glob(filepath.Join(migrationsPath, "*.sql"))
	if err != nil {
		t.Fatalf("list postgres migrations: %v", err)
	}

	var appliedCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT count(*) FROM public.schema_migrations WHERE schema_name = $1`,
		"tenant",
	).Scan(&appliedCount); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if appliedCount != len(expectedCount) {
		t.Fatalf("expected %d applied migrations, got %d", len(expectedCount), appliedCount)
	}

	var accountTable sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT to_regclass($1)`, qualifiedName("tenant", "identity_accounts")).Scan(&accountTable); err != nil {
		t.Fatalf("check identity accounts table: %v", err)
	}
	if !accountTable.Valid || accountTable.String == "" {
		t.Fatal("expected identity accounts table to exist")
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
		"--network",
		"host",
		"-e",
		"POSTGRES_PASSWORD=pass",
		"-e",
		"POSTGRES_DB=test",
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

	dsn := "postgres://postgres:pass@127.0.0.1:5432/test?sslmode=disable"
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
