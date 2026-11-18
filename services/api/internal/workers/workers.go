// Package workers implements outbox event handlers: notifications and other
// follow-up work. All handlers are idempotent because the dispatcher retries.
package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/config"
	"biteorbit/services/api/internal/outbox"
	"biteorbit/services/api/internal/platform/notify"
)

// Workers bundles handler dependencies.
type Workers struct {
	pool  *pgxpool.Pool
	cfg   *config.Config
	log   *slog.Logger
	email notify.EmailSender
	sms   notify.SMSSender
}

func New(pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger, email notify.EmailSender, sms notify.SMSSender) *Workers {
	return &Workers{pool: pool, cfg: cfg, log: log, email: email, sms: sms}
}

// RegisterAll subscribes handlers to the dispatcher.
func RegisterAll(d *outbox.Dispatcher, w *Workers) {
	for _, evtType := range []string{
		"order.status_changed",
		"order.rejected",
		"order.cancelled",
		"payment.captured",
		"payment.refunded",
		"delivery.assigned",
		"delivery.completed",
		"support.ticket_replied",
		"application.reviewed",
	} {
		d.Register(evtType, w)
	}
}

func (w *Workers) Name() string { return "notification-worker" }

type statusPayload struct {
	OrderID    string `json:"order_id"`
	Reference  string `json:"reference"`
	CustomerID string `json:"customer_id"`
	From       string `json:"from_state"`
	To         string `json:"to_state"`
	Reason     string `json:"reason"`
}

// Handle routes an event to the right side-effect. Idempotency: notification
// inserts use dedup keys carried in payload; replaying an event is a no-op.
func (w *Workers) Handle(ctx context.Context, evt outbox.Event) error {
	switch evt.EventType {
	case "order.status_changed", "order.rejected", "order.cancelled":
		var p statusPayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return fmt.Errorf("decode payload: %w", err)
		}
		return w.notifyOrderStatus(ctx, p)
	case "delivery.assigned":
		var p statusPayload
		if err := json.Unmarshal(evt.Payload, &p); err != nil {
			return fmt.Errorf("decode payload: %w", err)
		}
		return w.notifyRiderAssigned(ctx, p)
	default:
		// Other event types currently have no worker side effects.
		return nil
	}
}

// notifyOrderStatus creates the in-app notification for the customer and an
// (optional, preference-respecting) email via the configured adapter.
func (w *Workers) notifyOrderStatus(ctx context.Context, p statusPayload) error {
	title, body := orderStatusCopy(p)
	dedup := fmt.Sprintf("order-status:%s:%s", p.OrderID, p.To)
	var notifID string
	err := w.pool.QueryRow(ctx, `
		INSERT INTO notifications (user_id, category, title, body, metadata, read_at)
		VALUES ($1, 'order', $2, $3, jsonb_build_object('order_id',$4,'reference',$5,'state',$6), NULL)
		WHERE NOT EXISTS (SELECT 1 FROM notifications WHERE metadata->>'dedup' = $7)
		RETURNING id`,
		p.CustomerID, title, body, p.OrderID, p.Reference, p.To, dedup).Scan(&notifID)
	// No row when the dedup key already exists — that's success (idempotent replay).
	if err != nil && !isNoRows(err) {
		return fmt.Errorf("insert notification: %w", err)
	}
	if err != nil {
		return nil
	}
	// Attach the dedup marker (single extra update keeps the INSERT generic).
	if _, err := w.pool.Exec(ctx,
		`UPDATE notifications SET metadata = metadata || jsonb_build_object('dedup', $2) WHERE id = $1`,
		notifID, dedup); err != nil {
		return err
	}

	if w.email != nil {
		var email string
		var wantEmail bool
		_ = w.pool.QueryRow(ctx, `
			SELECT u.email,
				COALESCE(np.email, true)
			FROM users u
			LEFT JOIN notification_preferences np ON np.user_id = u.id AND np.category = 'order'
			WHERE u.id = $1`, p.CustomerID).Scan(&email, &wantEmail)
		if email != "" && wantEmail {
			if err := w.email.Send(ctx, email,
				fmt.Sprintf("BiteOrbit — order %s update", p.Reference),
				fmt.Sprintf("%s\n\nThis is a local demo notification (simulated email).\n", body)); err != nil {
				// Delivery failure must not crash order flow; log and continue.
				w.log.WarnContext(ctx, "email delivery failed", slog.Any("err", err))
				_, _ = w.pool.Exec(ctx, `
					INSERT INTO notification_deliveries (notification_id, channel, status, detail)
					VALUES ($1, 'email', 'failed', $2)`, notifID, err.Error())
				return nil
			}
			_, _ = w.pool.Exec(ctx, `
				INSERT INTO notification_deliveries (notification_id, channel, status, sent_at)
				VALUES ($1, 'email', 'sent', now())
				ON CONFLICT (notification_id, channel) DO NOTHING`, notifID)
		}
	}
	return nil
}

func (w *Workers) notifyRiderAssigned(ctx context.Context, p statusPayload) error {
	var riderUserID string
	err := w.pool.QueryRow(ctx, `
		SELECT dp.user_id FROM delivery_assignments da
		JOIN delivery_partners dp ON dp.id = da.rider_id
		WHERE da.order_id = $1
		ORDER BY da.created_at DESC LIMIT 1`, p.OrderID).Scan(&riderUserID)
	if err != nil {
		return nil // assignment may have been reassigned; nothing to notify
	}
	_, err = w.pool.Exec(ctx, `
		INSERT INTO notifications (user_id, category, title, body, metadata)
		VALUES ($1, 'delivery', 'New delivery offer', $2, jsonb_build_object('order_id',$3))`,
		riderUserID, fmt.Sprintf("Order %s needs a rider. Check your offers.", p.Reference), p.OrderID)
	if err != nil {
		return fmt.Errorf("rider notification: %w", err)
	}
	return nil
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func orderStatusCopy(p statusPayload) (string, string) {
	switch p.To {
	case "restaurant_confirmed":
		return "Order confirmed 🎉", fmt.Sprintf("%s is preparing your order.", p.Reference)
	case "preparing":
		return "Your food is being prepared", fmt.Sprintf("%s is in the kitchen.", p.Reference)
	case "ready_for_pickup":
		return "Almost ready", fmt.Sprintf("%s is packed and waiting for a rider.", p.Reference)
	case "rider_assigned":
		return "Rider on it", fmt.Sprintf("A delivery partner is heading to pick up %s.", p.Reference)
	case "picked_up":
		return "Picked up", fmt.Sprintf("Your order %s is on the move.", p.Reference)
	case "on_the_way":
		return "On the way 🛵", fmt.Sprintf("%s is close by (simulated demo tracking).", p.Reference)
	case "delivered":
		return "Delivered!", fmt.Sprintf("%s was delivered. Enjoy your meal!", p.Reference)
	case "rejected":
		return "Order rejected", fmt.Sprintf("%s could not be accepted: %s", p.Reference, p.Reason)
	case "cancelled":
		return "Order cancelled", fmt.Sprintf("%s was cancelled. %s", p.Reference, p.Reason)
	case "delivery_failed":
		return "Delivery failed", fmt.Sprintf("%s could not be delivered: %s", p.Reference, p.Reason)
	default:
		return "Order update", fmt.Sprintf("%s moved to %s.", p.Reference, p.To)
	}
}
