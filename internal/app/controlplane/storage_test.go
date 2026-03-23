package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformstorage "github.com/dm-vev/zvonilka/internal/platform/storage"
)

func TestBuildAppStorageReturnsNilWhenPostgresDisabled(t *testing.T) {
	t.Parallel()

	catalog, service, err := buildAppStorage(context.Background(), config.Configuration{})
	if err != nil {
		t.Fatalf("build app storage: %v", err)
	}
	if catalog != nil {
		t.Fatalf("expected nil catalog, got %#v", catalog)
	}
	if service != nil {
		t.Fatalf("expected nil identity service, got %#v", service)
	}
}

func TestBuildAppStorageJoinsCleanupErrorOnStartupFailure(t *testing.T) {
	originalBuilder := newStorageBuilder
	originalIdentityStore := newIdentityStore
	originalIdentityService := newIdentityService
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
		newIdentityStore = originalIdentityStore
		newIdentityService = originalIdentityService
	})

	startupErr := errors.New("identity store failed")
	closeErr := errors.New("catalog close failed")
	catalog, err := domainstorage.NewCatalog(
		&fakeRelationalProvider{
			name:         "primary",
			closeErr:     closeErr,
			capabilities: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
		},
	)
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

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
		},
		Storage: config.StorageConfig{
			PrimaryProvider: "primary",
			CacheProvider:   "cache",
			ObjectProvider:  "object",
			AuditProvider:   "audit",
			SearchProvider:  "search",
		},
	}

	_, _, gotErr := buildAppStorage(context.Background(), cfg)
	if gotErr == nil {
		t.Fatal("expected startup error")
	}
	if !errors.Is(gotErr, startupErr) {
		t.Fatalf("expected startup error to be preserved, got %v", gotErr)
	}
	if !errors.Is(gotErr, closeErr) {
		t.Fatalf("expected cleanup error to be preserved, got %v", gotErr)
	}
	if !strings.Contains(gotErr.Error(), "close storage catalog") {
		t.Fatalf("expected cleanup wrapper in error: %v", gotErr)
	}
}

func TestBuildAppStorageUsesConfiguredPrimaryProvider(t *testing.T) {
	correctDB := new(sql.DB)
	wrongDB := new(sql.DB)

	catalog, err := domainstorage.NewCatalog(
		&fakeRelationalProvider{
			name:         "wrong-primary",
			db:           wrongDB,
			capabilities: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
		},
		&fakeRelationalProvider{
			name:         "primary",
			db:           correctDB,
			capabilities: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
		},
		&fakeProvider{name: "cache"},
		&fakeProvider{name: "object"},
		&fakeProvider{name: "audit"},
		&fakeProvider{name: "search"},
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
	newIdentityStore = func(db *sql.DB, schema string) (domainidentity.Store, error) {
		if db != correctDB {
			return nil, fmt.Errorf("unexpected primary database selected for schema %q", schema)
		}

		return nil, nil
	}
	newIdentityService = func(domainidentity.Store, domainidentity.CodeSender, ...domainidentity.Option) (*domainidentity.Service, error) {
		return &domainidentity.Service{}, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
			},
		},
		Storage: config.StorageConfig{
			PrimaryProvider: "primary",
			CacheProvider:   "cache",
			ObjectProvider:  "object",
			AuditProvider:   "audit",
			SearchProvider:  "search",
		},
	}

	createdCatalog, service, gotErr := buildAppStorage(context.Background(), cfg)
	if gotErr != nil {
		t.Fatalf("build app storage: %v", gotErr)
	}
	if createdCatalog != catalog {
		t.Fatal("expected configured catalog")
	}
	if service == nil {
		t.Fatal("expected identity service")
	}
	if err := closeStorageCatalog(context.Background(), createdCatalog); err != nil {
		t.Fatalf("close storage catalog: %v", err)
	}
}

func TestCloseStorageCatalogClosesProvidersInReverseOrder(t *testing.T) {
	t.Parallel()

	order := make([]string, 0, 2)
	catalog, err := domainstorage.NewCatalog(
		&fakeProvider{name: "first", order: &order},
		&fakeProvider{name: "second", order: &order},
	)
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	if err := closeStorageCatalog(context.Background(), catalog); err != nil {
		t.Fatalf("close storage catalog: %v", err)
	}

	if want := []string{"second", "first"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected close order: got %v want %v", order, want)
	}
}

func TestCloseStorageCatalogReturnsProviderError(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("provider close failed")
	catalog, err := domainstorage.NewCatalog(&fakeProvider{name: "first", closeErr: closeErr})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	got := closeStorageCatalog(context.Background(), catalog)
	if got == nil {
		t.Fatal("expected close error")
	}
	if !errors.Is(got, closeErr) {
		t.Fatalf("expected provider close error in final error: %v", got)
	}
	if !strings.Contains(got.Error(), "close storage catalog") {
		t.Fatalf("expected wrapped catalog close error: %v", got)
	}
}

func TestFinalizeRunReturnsRunErrorUnchangedWhenCloseSucceeds(t *testing.T) {
	t.Parallel()

	runErr := errors.New("runtime failed")
	catalog, err := domainstorage.NewCatalog(&fakeProvider{name: "first"})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	got := finalizeRun(context.Background(), &app{catalog: catalog}, runErr)
	if got == nil {
		t.Fatal("expected runtime error")
	}
	if !errors.Is(got, runErr) {
		t.Fatalf("expected runtime error in final error: %v", got)
	}
	if strings.Contains(got.Error(), "close controlplane app") {
		t.Fatalf("did not expect close wrapper when close succeeded: %v", got)
	}
}

func TestFinalizeRunReturnsCloseErrorWhenRunSucceeds(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	catalog, err := domainstorage.NewCatalog(&fakeProvider{name: "primary", closeErr: closeErr})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	got := finalizeRun(context.Background(), &app{catalog: catalog}, nil)
	if got == nil {
		t.Fatal("expected close error")
	}
	if !errors.Is(got, closeErr) {
		t.Fatalf("expected close error in final error: %v", got)
	}
	if !strings.Contains(got.Error(), "close controlplane app") {
		t.Fatalf("expected wrapped close error: %v", got)
	}
}

func TestFinalizeRunJoinsRunAndCloseErrors(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	runErr := errors.New("runtime failed")
	catalog, err := domainstorage.NewCatalog(&fakeProvider{name: "primary", closeErr: closeErr})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	got := finalizeRun(context.Background(), &app{catalog: catalog}, runErr)
	if got == nil {
		t.Fatal("expected joined error")
	}
	if !errors.Is(got, runErr) {
		t.Fatalf("expected runtime error in final error: %v", got)
	}
	if !errors.Is(got, closeErr) {
		t.Fatalf("expected close error in final error: %v", got)
	}
	if !strings.Contains(got.Error(), runErr.Error()) {
		t.Fatalf("expected runtime error text in final error: %v", got)
	}
	if !strings.Contains(got.Error(), closeErr.Error()) {
		t.Fatalf("expected close error text in final error: %v", got)
	}
}

type fakeProvider struct {
	name     string
	closeErr error
	order    *[]string
}

func (p *fakeProvider) Name() string {
	return p.name
}

func (p *fakeProvider) Kind() domainstorage.Kind {
	return domainstorage.KindCustom
}

func (p *fakeProvider) Purpose() domainstorage.Purpose {
	return domainstorage.PurposeCustom
}

func (p *fakeProvider) Capabilities() domainstorage.Capability {
	return domainstorage.CapabilityTransactions
}

func (p *fakeProvider) Close(context.Context) error {
	if p.order != nil {
		*p.order = append(*p.order, p.name)
	}

	return p.closeErr
}

type fakeRelationalProvider struct {
	name         string
	db           *sql.DB
	closeErr     error
	capabilities domainstorage.Capability
}

func (p *fakeRelationalProvider) Name() string {
	return p.name
}

func (p *fakeRelationalProvider) Kind() domainstorage.Kind {
	return domainstorage.KindRelational
}

func (p *fakeRelationalProvider) Purpose() domainstorage.Purpose {
	return domainstorage.PurposePrimary
}

func (p *fakeRelationalProvider) Capabilities() domainstorage.Capability {
	return p.capabilities
}

func (p *fakeRelationalProvider) Close(context.Context) error {
	return p.closeErr
}

func (p *fakeRelationalProvider) DB() *sql.DB {
	return p.db
}

type fakeBuilder struct {
	catalog *domainstorage.Catalog
}

func (b *fakeBuilder) Build(context.Context) (*domainstorage.Catalog, error) {
	return b.catalog, nil
}
