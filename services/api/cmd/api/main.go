// Command api runs the BiteOrbit HTTP API server.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"biteorbit/services/api/internal/config"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/outbox"
	"biteorbit/services/api/internal/server"
	"biteorbit/services/api/internal/workers"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	cfg, err := config.FromEnv()
	if err != nil {
		log.Error("configuration invalid", slog.Any("err", err))
		os.Exit(1)
	}
	log.Info("starting BiteOrbit API",
		slog.String("env", string(cfg.Env)), slog.String("addr", cfg.HTTPAddr))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.ConnectWithRetry(ctx, cfg.DatabaseURL, 30, 2*time.Second, log)
	if err != nil {
		log.Error("database unavailable", slog.Any("err", err))
		os.Exit(1)
	}
	defer pool.Close()

	deps, err := server.BuildDeps(cfg, pool, log)
	if err != nil {
		log.Error("dependency wiring failed", slog.Any("err", err))
		os.Exit(1)
	}
	handler, _, err := server.New(deps)
	if err != nil {
		log.Error("server assembly failed", slog.Any("err", err))
		os.Exit(1)
	}

	// Outbox dispatcher: turns committed events into notifications/jobs.
	dispatcher := outbox.NewDispatcher(pool, log)
	workers.RegisterAll(dispatcher, workers.New(pool, cfg, log, deps.Email, deps.SMS))
	go dispatcher.Run()
	defer dispatcher.Stop()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		log.Error("listen failed", slog.String("addr", cfg.HTTPAddr), slog.Any("err", err))
		os.Exit(1)
	}
	go func() {
		log.Info("API listening", slog.String("addr", cfg.HTTPAddr))
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("graceful shutdown failed", slog.Any("err", err))
	}
}
