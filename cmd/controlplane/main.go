package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dm-vev/zvonilka/internal/app/controlplane"
)

func main() {
	if err := controlplane.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "controlplane: %v\n", err)
		os.Exit(1)
	}
}
