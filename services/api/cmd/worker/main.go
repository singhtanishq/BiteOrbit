// Command worker runs the outbox dispatcher in standalone mode (the API also
// runs one in-process; a separate worker lets deployments scale processing).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"biteorbit/services/api/internal/config"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/dispatch"
	"biteorbit/services/api/internal/outbox"
	"biteorbit/services/api/internal/platform/notify"
	"biteorbit/services/api/internal/workers"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.FromEnv()
	if err != nil {
		log.Error("configuration invalid", slog.Any("err", err))
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.ConnectWithRetry(ctx, cfg.DatabaseURL, 30, 2*time.Second, log)
	if err != nil {
		log.Error("database unavailable", slog.Any("err", err))
		os.Exit(1)
	}
	defer pool.Close()

	log.Info("worker starting", slog.String("env", string(cfg.Env)))
	dispatcher := outbox.NewDispatcher(pool, log)
	workers.RegisterAll(dispatcher, workers.New(pool, cfg, log,
		&notify.ConsoleEmail{Log: log}, &notify.FakeSMS{Log: log}), dispatch.NewHandler(pool, log))
	go dispatcher.Run()

	<-ctx.Done()
	log.Info("worker stopping")
	dispatcher.Stop()
}
