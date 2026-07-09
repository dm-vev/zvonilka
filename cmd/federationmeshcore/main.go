package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dm-vev/zvonilka/internal/app/federationmeshcore"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := federationmeshcore.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "federationmeshcore: %v\n", err)
		os.Exit(1)
	}
}
