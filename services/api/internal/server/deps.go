package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/auth"
	"biteorbit/services/api/internal/config"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/platform/notify"
)

// Deps bundles everything domain packages and workers need.
type Deps struct {
	Cfg   *config.Config
	Pool  *pgxpool.Pool
	Log   *slog.Logger
	Auth  *auth.Service
	Limit httpx.RateLimiter
	Email notify.EmailSender
	SMS   notify.SMSSender

	// Test hooks: allow integration tests to relax rate limits (0 = default).
	LoginRateLimit        int
	ResetRateLimit        int
	ResetConfirmRateLimit int
	RegisterRateLimit     int
}

// BuildDeps constructs the dependency graph based on configuration. Local
// adapters (mock payments, fake SMS, Mailpit/console email) are chosen here
// and validated by config.Validate so production can never boot with fakes.
func BuildDeps(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (Deps, error) {
	var email notify.EmailSender
	switch cfg.EmailAdapter {
	case "mailpit":
		email = &notify.MailpitEmail{Addr: cfg.SMTPAddr}
	case "console":
		email = &notify.ConsoleEmail{Log: log}
	default:
		log.Warn("unknown email adapter, falling back to console", slog.String("adapter", cfg.EmailAdapter))
		email = &notify.ConsoleEmail{Log: log}
	}

	sms := notify.SMSSender(&notify.FakeSMS{Log: log})

	// Rate limiting: Redis when available (shared across instances),
	// in-memory fallback for single-process local runs.
	var limiter httpx.RateLimiter = httpx.NewMemoryRateLimiter()
	if cfg.RedisURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if rl, err := httpx.NewRedisRateLimiter(ctx, cfg.RedisURL); err == nil {
			limiter = rl
			log.Info("rate limiting via Redis")
		} else {
			log.Warn("Redis unavailable, using in-memory rate limiting", slog.Any("err", err))
		}
		cancel()
	}

	return Deps{
		Cfg:   cfg,
		Pool:  pool,
		Log:   log,
		Auth:  auth.NewService(pool, cfg.CookieDomain, false, email),
		Limit: limiter,
		Email: email,
		SMS:   sms,
	}, nil
}
