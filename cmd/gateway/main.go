package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dm-vev/zvonilka/internal/app/gateway"
)

func main() {
	if err := gateway.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "gateway: %v\n", err)
		os.Exit(1)
	}
}
