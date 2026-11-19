package workers

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/outbox"
)

// OrderAutoTimeoutMinutes is the acceptance window for restaurants. Orders
// still sitting in `placed` after this are auto-rejected with a clear reason
// and the customer is refunded (captured online payments) by the same path
// as manual rejection. Documented in docs/architecture/state-machine.md.
const OrderAutoTimeoutMinutes = 15

// Janitor runs periodic maintenance: stale-order auto-rejection and expired
// idempotency-record pruning.
type Janitor struct {
	pool  *pgxpool.Pool
	log   *slog.Logger
	every time.Duration
	stop  chan struct{}
}

func NewJanitor(pool *pgxpool.Pool, log *slog.Logger) *Janitor {
	return &Janitor{pool: pool, log: log, every: 2 * time.Minute, stop: make(chan struct{})}
}

func (j *Janitor) Stop() { close(j.stop) }

func (j *Janitor) Run() {
	ticker := time.NewTicker(j.every)
	defer ticker.Stop()
	for {
		select {
		case <-j.stop:
			return
		case <-ticker.C:
			j.sweep(context.Background())
		}
	}
}

func (j *Janitor) sweep(ctx context.Context) {
	j.autoRejectStaleOrders(ctx)
	j.pruneExpiredIdempotency(ctx)
}

// autoRejectStaleOrders rejects `placed` orders older than the window.
func (j *Janitor) autoRejectStaleOrders(ctx context.Context) {
	rows, err := j.pool.Query(ctx, `
		SELECT o.id, o.reference, o.customer_id,
			COALESCE(pm.id::text, ''), COALESCE(pm.provider_reference, ''), pm.method, COALESCE(pm.status, ''), COALESCE(pm.amount_paise, 0)
		FROM orders o
		LEFT JOIN payments pm ON pm.order_id = o.id
		WHERE o.status = 'placed' AND o.placed_at < now() - ($1 || ' minutes')::interval
		FOR UPDATE OF o SKIP LOCKED`, itoa(OrderAutoTimeoutMinutes))
	if err != nil {
		j.log.WarnContext(ctx, "auto-reject query failed", slog.Any("err", err))
		return
	}
	type row struct {
		id, reference, customerID, paymentID, provRef, method, payStatus string
		amount                                                           int64
	}
	var stale []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.reference, &r.customerID, &r.paymentID, &r.provRef, &r.method, &r.payStatus, &r.amount) == nil {
			stale = append(stale, r)
		}
	}
	rows.Close()
	for _, o := range stale {
		tx, err := j.pool.Begin(ctx)
		if err != nil {
			return
		}
		reason := "Auto-rejected: no response within the acceptance window (demo policy)."
		if _, err := tx.Exec(ctx, `
			UPDATE orders SET status='rejected', rejection_reason=$2, closed_at=now(), updated_at=now()
			WHERE id=$1 AND status='placed'`, o.id, reason); err != nil {
			_ = tx.Rollback(ctx)
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, reason)
			VALUES ($1,'state_changed','placed','rejected','system',$2)`, o.id, reason); err != nil {
			_ = tx.Rollback(ctx)
			continue
		}
		// refund captured online payments; cancel pending/due ones
		if o.method != "cod" && o.payStatus == "captured" && o.paymentID != "" {
			if _, err := tx.Exec(ctx, `
				UPDATE payments SET status='refunded', updated_at=now() WHERE id=$1`, o.paymentID); err != nil {
				_ = tx.Rollback(ctx)
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO refunds (payment_id, order_id, amount_paise, status, reason, provider_reference, initiated_by, completed_at)
				VALUES ($1,$2,$3,'processed',$4,$5,'system',now())`,
				o.paymentID, o.id, o.amount, reason, o.provRef+"-auto"); err != nil {
				_ = tx.Rollback(ctx)
				continue
			}
		} else if o.paymentID != "" {
			if _, err := tx.Exec(ctx, `
				UPDATE payments SET status='cancelled', updated_at=now()
				WHERE id=$1 AND status IN ('pending','cod_due')`, o.paymentID); err != nil {
				_ = tx.Rollback(ctx)
				continue
			}
		}
		if err := outbox.Enqueue(ctx, tx, "order", o.id, "order.rejected", map[string]any{
			"order_id": o.id, "reference": o.reference, "customer_id": o.customerID,
			"reason": reason, "from_state": "placed", "to_state": "rejected",
		}, ptrStr("auto-reject:"+o.id)); err != nil {
			_ = tx.Rollback(ctx)
			continue
		}
		if err := tx.Commit(ctx); err != nil {
			j.log.WarnContext(ctx, "auto-reject commit failed", slog.Any("err", err))
			continue
		}
		j.log.InfoContext(ctx, "order auto-rejected",
			slog.String("reference", o.reference), slog.Int("window_minutes", OrderAutoTimeoutMinutes))
	}
}

// pruneExpiredIdempotency removes stale idempotency records.
func (j *Janitor) pruneExpiredIdempotency(ctx context.Context) {
	if _, err := j.pool.Exec(ctx,
		`DELETE FROM idempotency_records WHERE expires_at < now()`); err != nil {
		j.log.WarnContext(ctx, "idempotency prune failed", slog.Any("err", err))
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	s := ""
	for v > 0 {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	return s
}

func ptrStr(s string) *string { return &s }
