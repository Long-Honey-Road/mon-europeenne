package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
)

var errBadDirection = errors.New("want up or down")

func runMigrate(args []string, stdout io.Writer) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "down") {
		return errBadDirection
	}
	cfg, err := databaseConfig()
	if err != nil {
		return err
	}

	migrate := store.MigrateUp
	if args[0] == "down" {
		migrate = store.MigrateDown
	}
	if err := migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "migrated %s\n", args[0])
	return nil
}

// runSeed only checks the database for now; loading seed data comes with F-06.
func runSeed(_ []string, stdout io.Writer) error {
	cfg, err := databaseConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	fmt.Fprintln(stdout, "database reachable, nothing to seed yet")
	return nil
}

func databaseConfig() (config.Config, error) {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return config.Config{}, err
	}
	return cfg, cfg.RequireDatabase()
}
