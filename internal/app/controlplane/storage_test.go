package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	postgresdomain "github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
	identityteststore "github.com/dm-vev/zvonilka/internal/domain/identity/teststore"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	presenceteststore "github.com/dm-vev/zvonilka/internal/domain/presence/teststore"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformstorage "github.com/dm-vev/zvonilka/internal/platform/storage"
)

func testObjectStorageConfig() config.ObjectStorageConfig {
	return config.ObjectStorageConfig{
		Enabled:         true,
		Endpoint:        "http://127.0.0.1:9000",
		Region:          "us-east-1",
		Bucket:          "zvonilka-media",
		AccessKeyID:     "test-access",
		SecretAccessKey: "test-secret",
		ForcePathStyle:  true,
	}
}

func testStorageBindings() config.StorageConfig {
	return config.StorageConfig{
		PrimaryProvider: "primary",
		CacheProvider:   "cache",
		ObjectProvider:  "object",
		AuditProvider:   "audit",
		SearchProvider:  "search",
	}
}

func TestBuildAppStorageRejectsDisabledStorageStack(t *testing.T) {
	t.Parallel()

	_, _, _, _, _, err := buildAppStorage(context.Background(), config.Configuration{})
	if err == nil {
		t.Fatal("expected disabled storage stack to fail")
	}
	if !strings.Contains(err.Error(), "postgres and object storage are required for controlplane") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildAppStorageRejectsNilStorageBuilder(t *testing.T) {
	originalBuilder := newStorageBuilder
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
	})

	newStorageBuilder = func(config.Configuration, ...platformstorage.Factory) (storageBuilder, error) {
		return nil, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
				Schema:  "tenant",
			},
		},
		Storage: testStorageBindings(),
	}

	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	_, _, _, _, _, err := buildAppStorage(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected storage builder error")
	}
	if !strings.Contains(err.Error(), "configure storage builder") {
		t.Fatalf("expected builder error, got %v", err)
	}
}

func TestBuildAppStorageRejectsNilCatalog(t *testing.T) {
	originalBuilder := newStorageBuilder
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
	})

	newStorageBuilder = func(config.Configuration, ...platformstorage.Factory) (storageBuilder, error) {
		return &fakeBuilder{}, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
			},
		},
		Storage: testStorageBindings(),
	}

	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	_, _, _, _, _, err := buildAppStorage(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected catalog error")
	}
	if !strings.Contains(err.Error(), "build storage catalog") {
		t.Fatalf("expected catalog error, got %v", err)
	}
}

func TestBuildAppStorageRejectsNilIdentityStore(t *testing.T) {
	originalBuilder := newStorageBuilder
	originalStore := newIdentityStore
	originalService := newIdentityService
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
		newIdentityStore = originalStore
		newIdentityService = originalService
	})

	catalog, err := domainstorage.NewCatalog(
		&fakeRelationalProvider{
			name:         "primary",
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
		return nil, nil
	}
	newIdentityService = func(domainidentity.Store, domainidentity.CodeSender, ...domainidentity.Option) (*domainidentity.Service, error) {
		t.Fatal("identity service constructor should not be called")
		return nil, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
				Schema:  "tenant",
			},
		},
		Storage: testStorageBindings(),
	}

	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	_, _, _, _, _, err = buildAppStorage(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected identity store error")
	}
	if !strings.Contains(err.Error(), "construct postgres identity store") {
		t.Fatalf("expected store error, got %v", err)
	}
}

func TestBuildAppStorageRejectsNilIdentityService(t *testing.T) {
	originalBuilder := newStorageBuilder
	originalStore := newIdentityStore
	originalService := newIdentityService
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
		newIdentityStore = originalStore
		newIdentityService = originalService
	})

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	store, err := postgresdomain.New(db, "tenant")
	if err != nil {
		t.Fatalf("new postgres store: %v", err)
	}

	catalog, err := domainstorage.NewCatalog(
		&fakeRelationalProvider{
			name:         "primary",
			db:           db,
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
		return store, nil
	}
	newIdentityService = func(domainidentity.Store, domainidentity.CodeSender, ...domainidentity.Option) (*domainidentity.Service, error) {
		return nil, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
				Schema:  "tenant",
			},
		},
		Storage: testStorageBindings(),
	}

	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	_, _, _, _, _, err = buildAppStorage(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected identity service error")
	}
	if !strings.Contains(err.Error(), "construct identity service") {
		t.Fatalf("expected service error, got %v", err)
	}
}

func TestBuildAppStorageRejectsMissingPrimaryProvider(t *testing.T) {
	originalBuilder := newStorageBuilder
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
	})

	closeCtxErrs := make([]error, 0, 1)
	catalog, err := domainstorage.NewCatalog(
		&fakeRelationalProvider{
			name:         "secondary",
			capabilities: domainstorage.CapabilityRead | domainstorage.CapabilityWrite | domainstorage.CapabilityTransactions,
			closeCtxErrs: &closeCtxErrs,
		},
	)
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	newStorageBuilder = func(config.Configuration, ...platformstorage.Factory) (storageBuilder, error) {
		return &fakeBuilder{catalog: catalog}, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
			},
		},
		Storage: testStorageBindings(),
	}

	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	_, _, _, _, _, gotErr := buildAppStorage(context.Background(), cfg)
	if gotErr == nil {
		t.Fatal("expected provider selection error")
	}
	if !strings.Contains(gotErr.Error(), "select primary storage provider") {
		t.Fatalf("expected provider selection error, got %v", gotErr)
	}
	if len(closeCtxErrs) != 1 {
		t.Fatalf("expected one catalog close, got %d", len(closeCtxErrs))
	}
	if closeCtxErrs[0] != nil {
		t.Fatalf("expected cleanup context to ignore cancellation, got %v", closeCtxErrs[0])
	}
}

func TestBuildAppStorageRejectsNonRelationalPrimaryProvider(t *testing.T) {
	originalBuilder := newStorageBuilder
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
	})

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

	newStorageBuilder = func(config.Configuration, ...platformstorage.Factory) (storageBuilder, error) {
		return &fakeBuilder{catalog: catalog}, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
			},
		},
		Storage: testStorageBindings(),
	}

	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	_, _, _, _, _, gotErr := buildAppStorage(context.Background(), cfg)
	if gotErr == nil {
		t.Fatal("expected provider type error")
	}
	if !strings.Contains(gotErr.Error(), "expected relational provider") {
		t.Fatalf("expected provider type error, got %v", gotErr)
	}
	if len(closeCtxErrs) != 1 {
		t.Fatalf("expected one catalog close, got %d", len(closeCtxErrs))
	}
	if closeCtxErrs[0] != nil {
		t.Fatalf("expected cleanup context to ignore cancellation, got %v", closeCtxErrs[0])
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
		Storage: testStorageBindings(),
	}

	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	_, _, _, _, _, gotErr := buildAppStorage(context.Background(), cfg)
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
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	correctDB := db
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
		if schema != "tenant" {
			return nil, fmt.Errorf("unexpected schema %q", schema)
		}

		return postgresdomain.New(db, schema)
	}
	newIdentityService = func(domainidentity.Store, domainidentity.CodeSender, ...domainidentity.Option) (*domainidentity.Service, error) {
		return &domainidentity.Service{}, nil
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
				Schema:  "tenant",
			},
		},
		Storage: testStorageBindings(),
	}

	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	createdCatalog, service, conversationService, mediaService, presenceService, gotErr := buildAppStorage(context.Background(), cfg)
	if gotErr != nil {
		t.Fatalf("build app storage: %v", gotErr)
	}
	if createdCatalog != catalog {
		t.Fatal("expected configured catalog")
	}
	if service == nil {
		t.Fatal("expected identity service")
	}
	if conversationService == nil {
		t.Fatal("expected conversation service")
	}
	if mediaService == nil {
		t.Fatal("expected media service")
	}
	if presenceService == nil {
		t.Fatal("expected presence service")
	}
	if err := closeStorageCatalog(context.Background(), createdCatalog); err != nil {
		t.Fatalf("close storage catalog: %v", err)
	}
}

func TestBuildAppStorageUsesConfiguredPresenceSettings(t *testing.T) {
	originalBuilder := newStorageBuilder
	originalStore := newIdentityStore
	originalService := newIdentityService
	originalPresenceStore := newPresenceStore
	originalPresenceService := newPresenceService
	t.Cleanup(func() {
		newStorageBuilder = originalBuilder
		newIdentityStore = originalStore
		newIdentityService = originalService
		newPresenceStore = originalPresenceStore
		newPresenceService = originalPresenceService
	})

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	identityStore := identityteststore.NewMemoryStore()
	now := time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	if _, err := identityStore.SaveAccount(context.Background(), domainidentity.Account{
		ID:         "acc-1",
		Username:   "alice",
		Status:     domainidentity.AccountStatusActive,
		LastAuthAt: now.Add(-20 * time.Minute),
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}

	catalog, err := domainstorage.NewCatalog(
		&fakeRelationalProvider{
			name:         "primary",
			db:           db,
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

	newStorageBuilder = func(config.Configuration, ...platformstorage.Factory) (storageBuilder, error) {
		return &fakeBuilder{catalog: catalog}, nil
	}
	newIdentityStore = func(*sql.DB, string) (domainidentity.Store, error) {
		return identityStore, nil
	}
	newIdentityService = func(domainidentity.Store, domainidentity.CodeSender, ...domainidentity.Option) (*domainidentity.Service, error) {
		return &domainidentity.Service{}, nil
	}
	newPresenceStore = func(*sql.DB, string) (domainpresence.Store, error) {
		return presenceteststore.NewMemoryStore(), nil
	}
	newPresenceService = func(
		store domainpresence.Store,
		identityStore domainidentity.Store,
		opts ...domainpresence.Option,
	) (*domainpresence.Service, error) {
		return domainpresence.NewService(
			store,
			identityStore,
			append(opts, domainpresence.WithNow(func() time.Time { return now }))...,
		)
	}

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			Postgres: config.PostgresConfig{
				Enabled: true,
				Schema:  "tenant",
			},
		},
		Storage: testStorageBindings(),
		Presence: config.PresenceConfig{
			OnlineWindow: 30 * time.Minute,
		},
	}
	cfg.Infrastructure.ObjectStore = testObjectStorageConfig()

	_, _, _, _, presenceService, err := buildAppStorage(context.Background(), cfg)
	if err != nil {
		t.Fatalf("build app storage: %v", err)
	}
	if presenceService == nil {
		t.Fatal("expected presence service")
	}

	snapshot, err := presenceService.GetPresence(context.Background(), domainpresence.GetParams{AccountID: "acc-1"})
	if err != nil {
		t.Fatalf("get presence: %v", err)
	}
	if snapshot.State != domainpresence.PresenceStateOnline {
		t.Fatalf("expected configured online window to keep account online, got %s", snapshot.State)
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
	name         string
	bucket       string
	closeErr     error
	order        *[]string
	closeCtxErrs *[]error
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

func (p *fakeProvider) Close(ctx context.Context) error {
	return p.close(ctx)
}

func (p *fakeProvider) Bucket() string {
	if p.bucket != "" {
		return p.bucket
	}

	return p.name
}

func (p *fakeProvider) PutObject(context.Context, string, io.Reader, int64, domainstorage.PutObjectOptions) (domainstorage.BlobObject, error) {
	return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
}

func (p *fakeProvider) GetObject(context.Context, string) (io.ReadCloser, domainstorage.BlobObject, error) {
	return nil, domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
}

func (p *fakeProvider) HeadObject(context.Context, string) (domainstorage.BlobObject, error) {
	return domainstorage.BlobObject{}, domainstorage.ErrInvalidInput
}

func (p *fakeProvider) DeleteObject(context.Context, string) error {
	return domainstorage.ErrInvalidInput
}

func (p *fakeProvider) PresignPutObject(context.Context, string, time.Duration, domainstorage.PutObjectOptions) (domainstorage.PresignedRequest, error) {
	return domainstorage.PresignedRequest{}, domainstorage.ErrInvalidInput
}

func (p *fakeProvider) PresignGetObject(context.Context, string, time.Duration) (domainstorage.PresignedRequest, error) {
	return domainstorage.PresignedRequest{}, domainstorage.ErrInvalidInput
}

func (p *fakeProvider) close(ctx context.Context) error {
	if p.order != nil {
		*p.order = append(*p.order, p.name)
	}
	if p.closeCtxErrs != nil {
		*p.closeCtxErrs = append(*p.closeCtxErrs, ctx.Err())
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	return p.closeErr
}

type fakeRelationalProvider struct {
	name         string
	db           *sql.DB
	closeErr     error
	capabilities domainstorage.Capability
	closeCtxErrs *[]error
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

func (p *fakeRelationalProvider) Close(ctx context.Context) error {
	return p.close(ctx)
}

func (p *fakeRelationalProvider) close(ctx context.Context) error {
	if p.closeCtxErrs != nil {
		*p.closeCtxErrs = append(*p.closeCtxErrs, ctx.Err())
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}

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
