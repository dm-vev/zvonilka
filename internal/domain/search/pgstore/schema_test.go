package pgstore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dm-vev/zvonilka/internal/domain/search"
	platformpostgres "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	"github.com/dm-vev/zvonilka/internal/testsupport/dockermutex"
)

func TestSearchSchemaLifecycle(t *testing.T) {
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
		"0014.sql",
		"0015.sql",
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store, err := New(db, "tenant")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	document := search.Document{
		Scope:       search.SearchScopeUsers,
		EntityType:  "user",
		TargetID:    "acc-1",
		Title:       "Alice Example",
		Subtitle:    "alice",
		Snippet:     "owner",
		UserID:      "acc-1",
		AccountKind: "user",
		Metadata:    map[string]string{"bio": "owner"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	saved, err := store.UpsertDocument(context.Background(), document)
	if err != nil {
		t.Fatalf("upsert document: %v", err)
	}
	if saved.ID != "users:acc-1" {
		t.Fatalf("expected document id users:acc-1, got %s", saved.ID)
	}

	loaded, err := store.DocumentByScopeAndTarget(context.Background(), search.SearchScopeUsers, "acc-1")
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if loaded.TargetID != document.TargetID {
		t.Fatalf("expected target %s, got %s", document.TargetID, loaded.TargetID)
	}

	result, err := store.SearchDocuments(context.Background(), search.SearchParams{
		Query:           "alice",
		Scopes:          []search.SearchScope{search.SearchScopeUsers},
		UserIDs:         []string{"acc-1"},
		IncludeSnippets: true,
		Limit:           10,
	})
	if err != nil {
		t.Fatalf("search documents: %v", err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(result.Hits))
	}
	if result.Hits[0].Snippet == "" {
		t.Fatal("expected snippet in search result")
	}

	if err := store.DeleteDocument(context.Background(), search.SearchScopeUsers, "acc-1"); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	if _, err := store.DocumentByScopeAndTarget(context.Background(), search.SearchScopeUsers, "acc-1"); err != search.ErrNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestSearchSchemaConstraints(t *testing.T) {
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
		"0014.sql",
		"0015.sql",
	)
	if err := platformpostgres.ApplyMigrations(context.Background(), db, migrationsPath, "tenant"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		query      string
		constraint string
		args       []any
	}{
		{
			name:       "blank target id",
			query:      `INSERT INTO tenant.search_documents (scope, target_id, entity_type, title, search_text, created_at, updated_at) VALUES ('users', '', 'user', 'Alice', 'alice', $1, $2)`,
			constraint: "search_documents_scope_reference_check",
			args:       []any{now, now},
		},
		{
			name:       "invalid scope",
			query:      `INSERT INTO tenant.search_documents (scope, target_id, entity_type, title, search_text, created_at, updated_at) VALUES ('alien', 'doc-1', 'alien', 'Alice', 'alice', $1, $2)`,
			constraint: "search_documents_scope_allowed_check",
			args:       []any{now, now},
		},
		{
			name:       "blank search text",
			query:      `INSERT INTO tenant.search_documents (scope, target_id, entity_type, title, search_text, conversation_id, message_id, created_at, updated_at) VALUES ('messages', 'msg-1', 'message', 'Alice', '', 'conv-1', 'msg-1', $1, $2)`,
			constraint: "search_documents_search_text_check",
			args:       []any{now, now},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.ExecContext(context.Background(), tc.query, tc.args...)
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
}

func openDockerPostgres(t *testing.T) *sql.DB {
	t.Helper()

	unlock := dockermutex.Acquire(t, "search-postgres-integration")
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
