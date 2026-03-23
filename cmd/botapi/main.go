package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dm-vev/zvonilka/internal/app/botapi"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := botapi.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "botapi: %v\n", err)
		os.Exit(1)
	}
}
