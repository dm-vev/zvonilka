package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dm-vev/zvonilka/internal/app/botapi"
)

func main() {
	if err := botapi.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "botapi: %v\n", err)
		os.Exit(1)
	}
}
