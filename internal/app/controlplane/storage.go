package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	postgresconversation "github.com/dm-vev/zvonilka/internal/domain/conversation/pgstore"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	postgresdomain "github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformstorage "github.com/dm-vev/zvonilka/internal/platform/storage"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

type storageBuilder interface {
	Build(ctx context.Context) (*domainstorage.Catalog, error)
}

var newStorageBuilder = func(
	cfg config.Configuration,
	factories ...platformstorage.Factory,
) (storageBuilder, error) {
	return platformstorage.NewBuilder(cfg, factories...)
}

var newIdentityStore = func(db *sql.DB, schema string) (domainidentity.Store, error) {
	return postgresdomain.New(db, schema)
}

var newIdentityService = func(
	store domainidentity.Store,
	sender domainidentity.CodeSender,
	opts ...domainidentity.Option,
) (*domainidentity.Service, error) {
	return domainidentity.NewService(store, sender, opts...)
}

var newConversationStore = func(db *sql.DB, schema string) (domainconversation.Store, error) {
	return postgresconversation.New(db, schema)
}

var newConversationService = func(store domainconversation.Store, opts ...domainconversation.Option) (*domainconversation.Service, error) {
	return domainconversation.NewService(store, opts...)
}

func buildAppStorage(ctx context.Context, cfg config.Configuration) (*domainstorage.Catalog, *domainidentity.Service, *domainconversation.Service, error) {
	if !cfg.Infrastructure.Postgres.Enabled {
		return nil, nil, nil, nil
	}

	bootstrap := postgresplatform.NewBootstrap(cfg)
	builder, err := newStorageBuilder(
		cfg,
		postgresplatform.NewFactory(
			bootstrap,
			cfg.Storage.PrimaryProvider,
			domainstorage.KindRelational,
			domainstorage.PurposePrimary,
			domainstorage.CapabilityRead|domainstorage.CapabilityWrite|domainstorage.CapabilityTransactions,
		),
		postgresplatform.NewFactory(
			bootstrap,
			cfg.Storage.CacheProvider,
			domainstorage.KindCache,
			domainstorage.PurposeCache,
			domainstorage.CapabilityRead|domainstorage.CapabilityWrite|domainstorage.CapabilityKeyValue,
		),
		postgresplatform.NewFactory(
			bootstrap,
			cfg.Storage.ObjectProvider,
			domainstorage.KindObject,
			domainstorage.PurposeObject,
			domainstorage.CapabilityRead|domainstorage.CapabilityWrite|domainstorage.CapabilityBlob|domainstorage.CapabilityListing,
		),
		postgresplatform.NewFactory(
			bootstrap,
			cfg.Storage.AuditProvider,
			domainstorage.KindIndex,
			domainstorage.PurposeAudit,
			domainstorage.CapabilityWrite|domainstorage.CapabilityListing,
		),
		postgresplatform.NewFactory(
			bootstrap,
			cfg.Storage.SearchProvider,
			domainstorage.KindIndex,
			domainstorage.PurposeSearch,
			domainstorage.CapabilityRead|domainstorage.CapabilityWrite|domainstorage.CapabilityListing,
		),
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("configure storage builder: %w", err)
	}
	if builder == nil {
		return nil, nil, nil, fmt.Errorf("configure storage builder: %w", domainstorage.ErrInvalidInput)
	}

	catalog, err := builder.Build(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if catalog == nil {
		return nil, nil, nil, fmt.Errorf("build storage catalog: %w", domainstorage.ErrInvalidInput)
	}

	provider, err := catalog.Provider(cfg.Storage.PrimaryProvider)
	if err != nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider %q: %w", cfg.Storage.PrimaryProvider, err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	if provider == nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider: %w", domainstorage.ErrInvalidInput),
			closeStorageCatalog(ctx, catalog),
		)
	}

	relational, ok := provider.(domainstorage.RelationalProvider)
	if !ok {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider: expected relational provider"),
			closeStorageCatalog(ctx, catalog),
		)
	}

	store, err := newIdentityStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres identity store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	if store == nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres identity store: %w", domainidentity.ErrInvalidInput),
			closeStorageCatalog(ctx, catalog),
		)
	}

	service, err := newIdentityService(store, domainidentity.NoopCodeSender{}, domainidentity.WithSettings(cfg.Identity.ToSettings()))
	if err != nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("construct identity service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	if service == nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("construct identity service: %w", domainidentity.ErrInvalidInput),
			closeStorageCatalog(ctx, catalog),
		)
	}

	conversationStore, err := newConversationStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres conversation store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	if conversationStore == nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres conversation store: %w", domainconversation.ErrInvalidInput),
			closeStorageCatalog(ctx, catalog),
		)
	}

	conversationService, err := newConversationService(conversationStore)
	if err != nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("construct conversation service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	if conversationService == nil {
		return nil, nil, nil, joinStorageError(
			fmt.Errorf("construct conversation service: %w", domainconversation.ErrInvalidInput),
			closeStorageCatalog(ctx, catalog),
		)
	}

	return catalog, service, conversationService, nil
}

func closeStorageCatalog(ctx context.Context, catalog *domainstorage.Catalog) error {
	if catalog == nil {
		return nil
	}

	if err := catalog.Close(cleanupContext(ctx)); err != nil {
		return fmt.Errorf("close storage catalog: %w", err)
	}

	return nil
}

func joinStorageError(cause error, cleanupErr error) error {
	if cleanupErr == nil {
		return cause
	}
	if cause == nil {
		return cleanupErr
	}

	return errors.Join(cause, cleanupErr)
}
