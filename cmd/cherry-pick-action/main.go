package main

import (
	"context"
	"log"
	"os"

	"github.com/rancher/cherry-pick-action/internal/app"
)

func main() {
	ctx := context.Background()

	cfg, err := app.LoadConfig()
	if err != nil {
		log.Printf("failed to load config: %v", err)
		os.Exit(1)
	}

	runner, err := app.NewRunner(cfg)
	if err != nil {
		log.Printf("failed to create runner: %v", err)
		os.Exit(1)
	}

	if err := runner.Run(ctx); err != nil {
		log.Printf("cherry-pick action failed: %v", err)
		os.Exit(1)
	}
}
