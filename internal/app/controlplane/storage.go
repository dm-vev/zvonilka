package controlplane

import (
	"context"
	"errors"
	"fmt"

	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	postgresdomain "github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformstorage "github.com/dm-vev/zvonilka/internal/platform/storage"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

func buildAppStorage(ctx context.Context, cfg config.Configuration) (*domainstorage.Catalog, *domainidentity.Service, error) {
	if !cfg.Infrastructure.Postgres.Enabled {
		return nil, nil, nil
	}

	bootstrap := postgresplatform.NewBootstrap(cfg)
	builder, err := platformstorage.NewBuilder(
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
		return nil, nil, fmt.Errorf("configure storage builder: %w", err)
	}

	catalog, err := builder.Build(ctx)
	if err != nil {
		return nil, nil, err
	}

	provider, err := catalog.Select(domainstorage.PurposePrimary, domainstorage.CapabilityTransactions)
	if err != nil {
		return nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	relational, ok := provider.(domainstorage.RelationalProvider)
	if !ok {
		return nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider: expected relational provider"),
			closeStorageCatalog(ctx, catalog),
		)
	}

	store, err := postgresdomain.New(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, joinStorageError(
			fmt.Errorf("construct postgres identity store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	service, err := domainidentity.NewService(store, domainidentity.NoopCodeSender{}, domainidentity.WithSettings(cfg.Identity.ToSettings()))
	if err != nil {
		return nil, nil, joinStorageError(
			fmt.Errorf("construct identity service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	return catalog, service, nil
}

func closeStorageCatalog(ctx context.Context, catalog *domainstorage.Catalog) error {
	if catalog == nil {
		return nil
	}

	if err := catalog.Close(ctx); err != nil {
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
