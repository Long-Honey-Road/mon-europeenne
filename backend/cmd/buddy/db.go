package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/config"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/domain"
	"github.com/Long-Honey-Road/mon-europeenne/backend/internal/store"
	"github.com/Long-Honey-Road/mon-europeenne/backend/seed"
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

// runSeed loads teams and test users into the database; plan and documents come with their tasks.
func runSeed(_ []string, stdout io.Writer) error {
	cfg, err := databaseConfig()
	if err != nil {
		return err
	}
	data, err := seed.Load(seed.Files)
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	teams := data.Teams()
	var users []domain.User
	for _, c := range data.Credentials() {
		users = append(users, c.User)
	}
	if err := store.New(pool).SeedOrg(ctx, teams, users); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "seeded %d teams, %d users\n", len(teams), len(users))
	return nil
}

func databaseConfig() (config.Config, error) {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return config.Config{}, err
	}
	return cfg, cfg.RequireDatabase()
}
