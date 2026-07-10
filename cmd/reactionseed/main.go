package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dm-vev/zvonilka/internal/domain/media"
	postgresmedia "github.com/dm-vev/zvonilka/internal/domain/media/pgstore"
	"github.com/dm-vev/zvonilka/internal/domain/reaction/pgstore"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	postgresplatform "github.com/dm-vev/zvonilka/internal/platform/storage/postgres"
	s3platform "github.com/dm-vev/zvonilka/internal/platform/storage/s3"
	"github.com/dm-vev/zvonilka/internal/reactionseed"
)

func main() {
	manifestPath := flag.String("manifest", "assets/reactions/catalog.json", "reaction catalog JSON manifest")
	ownerAccountID := flag.String("owner-account-id", os.Getenv("ZVONILKA_REACTION_SEED_OWNER_ACCOUNT_ID"), "existing account that owns seeded media assets")
	replace := flag.Bool("replace", false, "deactivate catalog reactions absent from the manifest")
	flag.Parse()

	if *ownerAccountID == "" {
		log.Fatal("-owner-account-id or ZVONILKA_REACTION_SEED_OWNER_ACCOUNT_ID is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *manifestPath, *ownerAccountID, *replace); err != nil {
		log.Fatalf("reactionseed: %v", err)
	}
}

func run(ctx context.Context, manifestPath, ownerAccountID string, replace bool) error {
	cfg, err := config.Load("gateway")
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	postgresBootstrap := postgresplatform.NewBootstrap(cfg)
	db, err := postgresBootstrap.Open(ctx)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer postgresBootstrap.Close(context.Background())

	objectBootstrap := s3platform.NewBootstrap(cfg, s3platform.WithName(cfg.Storage.ObjectProvider))
	blob, err := objectBootstrap.Open(ctx)
	if err != nil {
		return fmt.Errorf("open object storage: %w", err)
	}
	defer objectBootstrap.Close(context.Background())

	mediaStore, err := postgresmedia.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return fmt.Errorf("construct media store: %w", err)
	}
	mediaService, err := media.NewService(mediaStore, blob, media.WithSettings(cfg.Media.ToSettings()))
	if err != nil {
		return fmt.Errorf("construct media service: %w", err)
	}
	reactionStore, err := pgstore.New(db, cfg.Infrastructure.Postgres.Schema)
	if err != nil {
		return fmt.Errorf("construct reaction store: %w", err)
	}
	seeder, err := reactionseed.New(mediaService, mediaStore, reactionStore, ownerAccountID)
	if err != nil {
		return fmt.Errorf("construct reaction seeder: %w", err)
	}
	catalog, err := seeder.Seed(ctx, manifestPath, replace)
	if err != nil {
		return err
	}

	log.Printf("reaction catalog seeded: version=%s default=%s reactions=%d", catalog.Version, catalog.DefaultEmoji, len(catalog.Reactions))
	return nil
}
