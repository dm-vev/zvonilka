package botapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	domainbot "github.com/dm-vev/zvonilka/internal/domain/bot"
	botpg "github.com/dm-vev/zvonilka/internal/domain/bot/pgstore"
	domainconversation "github.com/dm-vev/zvonilka/internal/domain/conversation"
	conversationpg "github.com/dm-vev/zvonilka/internal/domain/conversation/pgstore"
	domainidentity "github.com/dm-vev/zvonilka/internal/domain/identity"
	identitypg "github.com/dm-vev/zvonilka/internal/domain/identity/pgstore"
	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	platformstorage "github.com/dm-vev/zvonilka/internal/platform/storage"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
)

type bootstrapCloser interface {
	Close(context.Context) error
}

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
	return identitypg.New(db, schema)
}

var newConversationStore = func(db *sql.DB, schema string) (domainconversation.Store, error) {
	return conversationpg.New(db, schema)
}

var newBotStore = func(db *sql.DB, schema string) (domainbot.Store, error) {
	return botpg.New(db, schema)
}

func buildAppStorage(
	ctx context.Context,
	cfg config.Configuration,
) (
	*domainstorage.Catalog,
	bootstrapCloser,
	*domainbot.Service,
	*domainbot.Worker,
	error,
) {
	if !cfg.Infrastructure.Postgres.Enabled {
		return nil, nil, nil, nil, fmt.Errorf("postgres is required for botapi: %w", domainstorage.ErrInvalidInput)
	}

	postgresBootstrap := postgresplatform.NewBootstrap(cfg)
	builder, err := newStorageBuilder(
		cfg,
		postgresplatform.NewFactory(
			postgresBootstrap,
			cfg.Storage.PrimaryProvider,
			domainstorage.KindRelational,
			domainstorage.PurposePrimary,
			domainstorage.CapabilityRead|domainstorage.CapabilityWrite|domainstorage.CapabilityTransactions,
		),
	)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("configure storage builder: %w", err)
	}
	if builder == nil {
		return nil, nil, nil, nil, fmt.Errorf("configure storage builder: %w", domainstorage.ErrInvalidInput)
	}

	catalog, err := builder.Build(ctx)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if catalog == nil {
		return nil, nil, nil, nil, fmt.Errorf("build storage catalog: %w", domainstorage.ErrInvalidInput)
	}

	provider, err := catalog.Provider(cfg.Storage.PrimaryProvider)
	if err != nil {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider %q: %w", cfg.Storage.PrimaryProvider, err),
			catalog.Close(ctx),
		)
	}

	relational, ok := provider.(domainstorage.RelationalProvider)
	if !ok {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("select primary storage provider: expected relational provider"),
			catalog.Close(ctx),
		)
	}

	identityStore, err := newIdentityStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres identity store: %w", err),
			catalog.Close(ctx),
		)
	}
	conversationStore, err := newConversationStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres conversation store: %w", err),
			catalog.Close(ctx),
		)
	}
	botStore, err := newBotStore(relational.DB(), cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct postgres bot store: %w", err),
			catalog.Close(ctx),
		)
	}

	identityService, err := domainidentity.NewService(
		identityStore,
		domainidentity.NoopCodeSender{},
		domainidentity.WithSettings(cfg.Identity.ToSettings()),
	)
	if err != nil {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct identity service: %w", err),
			catalog.Close(ctx),
		)
	}
	conversationService, err := domainconversation.NewService(conversationStore)
	if err != nil {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct conversation service: %w", err),
			catalog.Close(ctx),
		)
	}
	botService, err := domainbot.NewService(
		botStore,
		identityService,
		conversationService,
		conversationStore,
		domainbot.WithSettings(cfg.Bot.ToSettings()),
	)
	if err != nil {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct bot service: %w", err),
			catalog.Close(ctx),
		)
	}
	worker, err := domainbot.NewWorker(botService, nil)
	if err != nil {
		return nil, nil, nil, nil, joinStorageError(
			fmt.Errorf("construct bot worker: %w", err),
			catalog.Close(ctx),
		)
	}

	return catalog, postgresBootstrap, botService, worker, nil
}

func joinStorageError(runErr error, closeErr error) error {
	if closeErr == nil {
		return runErr
	}
	if runErr == nil {
		return closeErr
	}

	return errors.Join(runErr, fmt.Errorf("close botapi storage catalog: %w", closeErr))
}
