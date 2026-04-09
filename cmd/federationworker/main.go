package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dm-vev/zvonilka/internal/app/federationworker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := federationworker.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "federationworker: %v\n", err)
		os.Exit(1)
	}
}
