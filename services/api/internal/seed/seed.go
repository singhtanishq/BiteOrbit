// Package seed builds the deterministic synthetic BiteOrbit demo dataset.
//
// Everything here is synthetic: the city (Bitepur), localities, restaurants,
// people, orders, reviews and tickets. Credentials are demo-only and clearly
// documented. Seeding is refused in production environments.
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/auth"
)

// DemoPassword is the shared local demo password for all seeded accounts.
// It is safe ONLY because seeding is refused outside local/test environments.
const DemoPassword = "Demo@1234"

// SeedOptions controls what gets built.
type SeedOptions struct {
	Orders int // historical orders to synthesise
}

// DefaultOptions returns the standard demo dataset size.
func DefaultOptions() SeedOptions { return SeedOptions{Orders: 180} }

// Run truncates all business data and rebuilds the demo dataset deterministically.
func Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, opts SeedOptions) error {
	if opts.Orders < 10 {
		opts.Orders = 10
	}
	rng := rand.New(rand.NewSource(20261119)) // deterministic
	hash, err := auth.HashPassword(DemoPassword)
	if err != nil {
		return fmt.Errorf("hash demo password: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err := truncateAll(ctx, tx); err != nil {
		return fmt.Errorf("clear existing data: %w", err)
	}

	s := &seeder{
		ctx:        ctx,
		tx:         tx,
		rng:        rng,
		hash:       hash,
		log:        log,
		outletOrg:  map[string]string{},
		outletName: map[string]string{},
		zoneName:   map[string]string{},
		zoneCenter: map[string][2]float64{},
		itemIDs:    map[string][]string{},
	}
	for _, step := range []struct {
		name string
		fn   func() error
	}{
		{"platform config", s.seedPlatform},
		{"city & zones", s.seedCity},
		{"cuisines", s.seedCuisines},
		{"people", s.seedPeople},
		{"restaurants & menus", s.seedRestaurants},
		{"coupons", s.seedCoupons},
		{"orders & activity", func() error { return s.seedActivity(opts.Orders) }},
		{"support", s.seedSupport},
	} {
		if err := step.fn(); err != nil {
			return fmt.Errorf("seed %s: %w", step.name, err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seed: %w", err)
	}
	log.Info("seed complete",
		slog.Int("orders", opts.Orders),
		slog.String("demo_password", DemoPassword),
		slog.String("scope", "LOCAL DEMO ONLY — synthetic data"))
	return nil
}

func truncateAll(ctx context.Context, tx pgx.Tx) error {
	tables := []string{
		"rider_settlements", "restaurant_settlements",
		"rider_earnings", "delivery_locations", "delivery_events", "delivery_assignments",
		"rider_applications", "delivery_partners",
		"review_reports", "reviews", "favourites", "search_history",
		"support_attachments", "support_messages", "support_tickets",
		"notification_deliveries", "notification_preferences", "notifications",
		"outbox_events", "audit_logs", "idempotency_records", "feature_flags", "platform_settings",
		"refunds", "payment_attempts", "payment_webhook_events", "payments",
		"coupon_redemptions",
		"order_item_modifiers", "order_items", "order_addresses", "order_events", "orders",
		"coupons",
		"pricing_quotes", "pricing_rules", "commission_rules",
		"cart_item_modifiers", "cart_items", "carts",
		"item_modifier_groups", "modifier_options", "modifier_groups", "menu_items", "menu_categories",
		"outlet_collections", "outlet_cuisines", "outlet_holidays", "outlet_hours", "outlet_service_areas",
		"outlet_staff", "outlets", "application_events", "restaurant_applications", "restaurant_organisations",
		"cuisine_types", "service_zones",
		"customer_addresses", "customer_preferences",
		"auth_events", "sessions", "credentials", "user_roles", "users",
	}
	for _, t := range tables {
		if _, err := tx.Exec(ctx, "DELETE FROM "+t); err != nil {
			return fmt.Errorf("clear %s: %w", t, err)
		}
	}
	return nil
}

type seeder struct {
	ctx  context.Context
	tx   pgx.Tx
	rng  *rand.Rand
	hash string
	log  *slog.Logger

	// IDs populated across phases.
	adminID      string
	opsManagerID string
	supportID    string
	customerIDs  []string
	outletIDs    []string
	outletOrg    map[string]string
	outletName   map[string]string
	zoneIDs      []string
	zoneName     map[string]string
	zoneCenter   map[string][2]float64
	itemIDs      map[string][]string // outlet -> item ids
}

// insert runs an INSERT ... RETURNING id and scans the single uuid.
func (s *seeder) insert(query string, args ...any) (string, error) {
	var id string
	if err := s.tx.QueryRow(s.ctx, query, args...).Scan(&id); err != nil {
		return "", fmt.Errorf("%s: %w", firstLine(query), err)
	}
	return id, nil
}

// exec runs a statement, wrapping errors with the query's first line so a
// failed seed names the offending statement immediately.
func (s *seeder) exec(query string, args ...any) error {
	if _, err := s.tx.Exec(s.ctx, query, args...); err != nil {
		return fmt.Errorf("%s: %w", firstLine(query), err)
	}
	return nil
}

func (s *seeder) mustInsert(query string, args ...any) string {
	id, err := s.insert(query, args...)
	if err != nil {
		panic(err) // seeding is deterministic; a failure is a programming error
	}
	return id
}

func firstLine(q string) string {
	// Queries are written as backtick strings that start with a newline and
	// indentation; return the first non-empty, trimmed line instead.
	start := 0
	for start < len(q) {
		// skip leading whitespace
		for start < len(q) && (q[start] == '\n' || q[start] == ' ' || q[start] == '\t') {
			start++
		}
		end := start
		for end < len(q) && q[end] != '\n' {
			end++
		}
		line := q[start:end]
		if line != "" {
			return line
		}
		start = end + 1
	}
	return q
}
