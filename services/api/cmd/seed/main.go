// Command seed rebuilds the synthetic BiteOrbit demo dataset.
// Refuses to run against production environments (guard: BITEORBIT_ENV).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"biteorbit/services/api/internal/config"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/seed"
)

func main() {
	orders := flag.Int("orders", 180, "number of historical orders to synthesise")
	flag.Parse()

	cfg, err := config.FromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration invalid:", err)
		os.Exit(1)
	}
	if cfg.Env == config.EnvProduction {
		fmt.Fprintln(os.Stderr, "refusing to seed a production environment")
		os.Exit(1)
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database unreachable:", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := seed.Run(ctx, pool, log, seed.SeedOptions{Orders: *orders}); err != nil {
		fmt.Fprintln(os.Stderr, "seed failed:", err)
		os.Exit(1)
	}
}
