package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dm-vev/zvonilka/internal/app/federationmeshtastic"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := federationmeshtastic.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "federationmeshtastic: %v\n", err)
		os.Exit(1)
	}
}
