package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformstorage "github.com/dm-vev/zvonilka/internal/platform/storage"
)

func TestBuildAppStorageCleansUpCatalogWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	startupErr := errors.New("identity store failed")
	closeCtxErrs := make([]error, 0, 1)
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	catalog, err := domainstorage.NewCatalog(
		&fakeRelationalProvider{
			name:         "primary",
			db:           db,
			capabilities: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
			closeCtxErrs: &closeCtxErrs,
		},
		&fakeRelationalProvider{
			name:         "search",
			db:           db,
			capabilities: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityListing,
		},
	)
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	originalBuilder := newStorageBuilder
	originalIdentityStore := newIdentityStore
	originalIdentityService := newIdentityService
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
		newIdentityStore = originalIdentityStore
		newIdentityService = originalIdentityService
	})

	newStorageBuilder = func(config.Configuration, ...platformstorage.Factory) (storageBuilder, error) {
		return &fakeBuilder{catalog: catalog}, nil
	}
	newIdentityStore = func(*sql.DB, string) (domainidentity.Store, error) {
		return nil, startupErr
	}
	newIdentityService = func(domainidentity.Store, domainidentity.CodeSender, ...domainidentity.Option) (*domainidentity.Service, error) {
		t.Fatal("identity service constructor should not be called")
		return nil, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
			},
			ObjectStore: testObjectStorageConfig(),
		},
		Storage: testStorageBindings(),
		Search:  testSearchConfig(),
	}

	_, _, _, _, _, _, gotErr := buildAppStorage(ctx, cfg)
	if gotErr == nil {
		t.Fatal("expected startup error")
	}
	if !errors.Is(gotErr, startupErr) {
		t.Fatalf("expected startup error to be preserved, got %v", gotErr)
	}
	if len(closeCtxErrs) != 1 {
		t.Fatalf("expected one catalog close, got %d", len(closeCtxErrs))
	}
	if closeCtxErrs[0] != nil {
		t.Fatalf("expected cleanup context to ignore cancellation, got %v", closeCtxErrs[0])
	}
}

func TestFinalizeRunCleansUpCatalogWithCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	closeCtxErrs := make([]error, 0, 1)
	catalog, err := domainstorage.NewCatalog(
		&fakeProvider{
			name:         "primary",
			closeCtxErrs: &closeCtxErrs,
		},
	)
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	gotErr := finalizeRun(ctx, &app{catalog: catalog}, nil)
	if gotErr != nil {
		t.Fatalf("expected clean shutdown, got %v", gotErr)
	}
	if len(closeCtxErrs) != 1 {
		t.Fatalf("expected one app close, got %d", len(closeCtxErrs))
	}
	if closeCtxErrs[0] != nil {
		t.Fatalf("expected cleanup context to ignore cancellation, got %v", closeCtxErrs[0])
	}
}

func TestFinalizeRunReturnsCloseErrorWithCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	closeErr := errors.New("catalog close failed")
	closeCtxErrs := make([]error, 0, 1)
	catalog, err := domainstorage.NewCatalog(
		&fakeProvider{
			name:         "primary",
			closeErr:     closeErr,
			closeCtxErrs: &closeCtxErrs,
		},
	)
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	gotErr := finalizeRun(ctx, &app{catalog: catalog}, nil)
	if gotErr == nil {
		t.Fatal("expected close error")
	}
	if !errors.Is(gotErr, closeErr) {
		t.Fatalf("expected close error to be preserved, got %v", gotErr)
	}
	if len(closeCtxErrs) != 1 {
		t.Fatalf("expected one app close, got %d", len(closeCtxErrs))
	}
	if closeCtxErrs[0] != nil {
		t.Fatalf("expected cleanup context to ignore cancellation, got %v", closeCtxErrs[0])
	}
}
