package gateway

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	postgresconversation "github.com/dm-vev/zvonilka/internal/domain/conversation/pgstore"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	postgresidentity "github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
	domainmedia "github.com/dm-vev/zvonilka/internal/domain/media"
	postgresmedia "github.com/dm-vev/zvonilka/internal/domain/media/pgstore"
	domainpresence "github.com/dm-vev/zvonilka/internal/domain/presence"
	postgrespresence "github.com/dm-vev/zvonilka/internal/domain/presence/pgstore"
	domainsearch "github.com/dm-vev/zvonilka/internal/domain/search"
	postgressearch "github.com/dm-vev/zvonilka/internal/domain/search/pgstore"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	domainuser "github.com/dm-vev/zvonilka/internal/domain/user"
	postgresuser "github.com/dm-vev/zvonilka/internal/domain/user/pgstore"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformstorage "github.com/dm-vev/zvonilka/internal/platform/storage"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	s3platform "github.com/dm-vev/zvonilka/internal/platform/storage/s3"
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
	return postgresidentity.New(db, schema)
}

var newConversationStore = func(db *sql.DB, schema string) (domainconversation.Store, error) {
	return postgresconversation.New(db, schema)
}

var newMediaStore = func(db *sql.DB, schema string) (domainmedia.Store, error) {
	return postgresmedia.New(db, schema)
}

var newPresenceStore = func(db *sql.DB, schema string) (domainpresence.Store, error) {
	return postgrespresence.New(db, schema)
}

var newSearchStore = func(db *sql.DB, schema string) (domainsearch.Store, error) {
	return postgressearch.New(db, schema)
}

var newUserStore = func(db *sql.DB, schema string) (domainuser.Store, error) {
	return postgresuser.New(db, schema)
}

func buildAppStorage(
	ctx context.Context,
	cfg config.Configuration,
) (
	*domainstorage.Catalog,
	*domainidentity.Service,
	*domainconversation.Service,
	*domainmedia.Service,
	*domainpresence.Service,
	*domainsearch.Service,
	*domainuser.Service,
	error,
) {
	if !cfg.Infrastructure.Postgres.Enabled || !cfg.Infrastructure.ObjectStore.Enabled {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf(
			"postgres and object storage are required for gateway: %w",
			domainstorage.ErrInvalidInput,
		)
	}

	postgresBootstrap := postgresplatform.NewBootstrap(cfg)
	objectBootstrap := s3platform.NewBootstrap(cfg)
	builder, err := newStorageBuilder(
		cfg,
		postgresplatform.NewFactory(
			postgresBootstrap,
			cfg.Storage.PrimaryProvider,
			domainstorage.KindRelational,
			domainstorage.PurposePrimary,
			domainstorage.CapabilityRead|domainstorage.CapabilityWrite|domainstorage.CapabilityTransactions,
		),
		postgresplatform.NewFactory(
			postgresBootstrap,
			cfg.Storage.SearchProvider,
			domainstorage.KindIndex,
			domainstorage.PurposeSearch,
			domainstorage.CapabilityRead|domainstorage.CapabilityWrite|domainstorage.CapabilityListing,
		),
		s3platform.NewFactory(
			objectBootstrap,
			cfg.Storage.ObjectProvider,
			domainstorage.KindObject,
			domainstorage.PurposeObject,
			domainstorage.CapabilityRead|domainstorage.CapabilityWrite|domainstorage.CapabilityBlob|domainstorage.CapabilityListing,
		),
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("configure storage builder: %w", err)
	}
	if builder == nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("configure storage builder: %w", domainstorage.ErrInvalidInput)
	}

	catalog, err := builder.Build(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	if catalog == nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("build storage catalog: %w", domainstorage.ErrInvalidInput)
	}

	provider, err := catalog.Provider(cfg.Storage.PrimaryProvider)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider %q: %w", cfg.Storage.PrimaryProvider, err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	relational, ok := provider.(domainstorage.RelationalProvider)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider: expected relational provider"),
			closeStorageCatalog(ctx, catalog),
		)
	}

	searchProvider, err := catalog.Provider(cfg.Storage.SearchProvider)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("select search storage provider %q: %w", cfg.Storage.SearchProvider, err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	searchRelational, ok := searchProvider.(domainstorage.RelationalProvider)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("select search storage provider: expected relational provider"),
			closeStorageCatalog(ctx, catalog),
		)
	}

	searchStore, err := newSearchStore(searchRelational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres search store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	searchService, err := domainsearch.NewService(searchStore, domainsearch.WithSettings(cfg.Search.ToSettings()))
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct search service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	identityStore, err := newIdentityStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres identity store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	identityService, err := domainidentity.NewService(
		identityStore,
		domainidentity.NoopCodeSender{},
		domainidentity.WithSettings(cfg.Identity.ToSettings()),
		domainidentity.WithIndexer(searchService),
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct identity service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	userStore, err := newUserStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres user store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	userService, err := domainuser.NewService(userStore, identityService)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct user service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	conversationStore, err := newConversationStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres conversation store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	conversationService, err := domainconversation.NewService(
		conversationStore,
		domainconversation.WithIndexer(searchService),
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct conversation service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	presenceStore, err := newPresenceStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres presence store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	presenceService, err := domainpresence.NewService(
		presenceStore,
		identityStore,
		domainpresence.WithSettings(cfg.Presence.ToSettings()),
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct presence service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	objectProvider, err := catalog.Provider(cfg.Storage.ObjectProvider)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("select object storage provider %q: %w", cfg.Storage.ObjectProvider, err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	blobStore, ok := objectProvider.(domainstorage.BlobStore)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("select object storage provider: expected blob store"),
			closeStorageCatalog(ctx, catalog),
		)
	}

	mediaStore, err := newMediaStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres media store: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}
	mediaService, err := domainmedia.NewService(
		mediaStore,
		blobStore,
		domainmedia.WithSettings(cfg.Media.ToSettings()),
		domainmedia.WithIndexer(searchService),
	)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct media service: %w", err),
			closeStorageCatalog(ctx, catalog),
		)
	}

	return catalog, identityService, conversationService, mediaService, presenceService, searchService, userService, nil
}

func closeStorageCatalog(ctx context.Context, catalog *domainstorage.Catalog) error {
	if catalog == nil {
		return nil
	}

	closeCtx, cancel := cleanupContext(ctx)
	defer cancel()

	return catalog.Close(closeCtx)
}

func joinStorageError(primary error, cleanup error) error {
	if cleanup == nil {
		return primary
	}
	if primary == nil {
		return cleanup
	}

	return errors.Join(primary, cleanup)
}
