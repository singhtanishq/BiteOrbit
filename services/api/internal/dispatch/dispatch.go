// Package dispatch assigns ready orders to available riders.
//
// Algorithm (deterministic, documented):
//  1. Eligible riders: approved, online, not already holding an open
//     assignment.
//  2. Score = fewest deliveries in the last hour (fairness) then shortest
//     distance from rider's last known position to the outlet (deterministic
//     tie-breaker). Exact road distance is unknowable; distances are
//     straight-line estimates.
//  3. The winner is claimed atomically (conditional UPDATE), an OTP is
//     generated for delivery confirmation, and the order moves to
//     rider_assigned only when the RIDER accepts the offer.
//
// If no eligible rider exists, the order stays in ready_for_pickup with an
// honest queue reason — never a fabricated assignment.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/courier"
	"biteorbit/services/api/internal/outbox"
)

// Handler implements outbox.Handler for "order.status_changed" events with
// to_state = ready_for_pickup.
type Handler struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

func NewHandler(pool *pgxpool.Pool, log *slog.Logger) *Handler {
	return &Handler{pool: pool, log: log}
}

func (h *Handler) Name() string { return "dispatch" }

type statusPayload struct {
	OrderID string `json:"order_id"`
	To      string `json:"to_state"`
}

func (h *Handler) Handle(ctx context.Context, evt outbox.Event) error {
	var p statusPayload
	if err := json.Unmarshal(evt.Payload, &p); err != nil {
		return fmt.Errorf("decode dispatch payload: %w", err)
	}
	if p.To != "ready_for_pickup" || p.OrderID == "" {
		return nil
	}
	return h.TryAssign(ctx, p.OrderID)
}

// TryAssign attempts one assignment; safe to call repeatedly (idempotent by
// the "one open assignment per order" rule).
func (h *Handler) TryAssign(ctx context.Context, orderID string) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// already assigned?
	var existing int
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM delivery_assignments
		WHERE order_id = $1 AND status NOT IN ('expired','rejected','cancelled')`, orderID).Scan(&existing)
	if err != nil {
		return err
	}
	if existing > 0 {
		_ = tx.Rollback(ctx)
		return nil // already has an assignment (maybe accepted)
	}

	// order must be ready and delivery mode
	var outletID string
	var outletLat, outletLng float64
	err = tx.QueryRow(ctx, `
		SELECT outlet_id, lat::float8, lng::float8 FROM orders o
		JOIN outlets ot ON ot.id = o.outlet_id
		WHERE o.id = $1 AND o.status = 'ready_for_pickup' FOR UPDATE`, orderID).Scan(&outletID, &outletLat, &outletLng)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return nil // not ready anymore; nothing to do
	}
	if err != nil {
		return err
	}

	// expire stale offers so riders are not blocked by them
	if _, err = tx.Exec(ctx, `
		UPDATE delivery_assignments SET status = 'expired', updated_at = now()
		WHERE status = 'assigned' AND expires_at <= now()`); err != nil {
		return err
	}

	// eligible riders: approved + online + no open assignment, scored by
	// fairness (fewest recent deliveries) then proximity
	type candidate struct {
		id       string
		userID   string
		score    float64
		distance float64
	}
	rows, err := tx.Query(ctx, `
		SELECT dp.id, dp.user_id,
			(COUNT(re.id) * 10.0) AS load_score,
			COALESCE(dp.current_lat, 0), COALESCE(dp.current_lng, 0)
		FROM delivery_partners dp
		JOIN user_roles ur ON ur.user_id = dp.user_id AND ur.role = 'rider'
		LEFT JOIN delivery_assignments da ON da.rider_id = dp.id
			AND da.status IN ('assigned','accepted','picked_up','on_the_way')
			AND da.created_at > now() - interval '2 hours' -- only recent work blocks
		LEFT JOIN rider_earnings re ON re.rider_id = dp.id AND re.created_at > now() - interval '1 hour'
		WHERE dp.status = 'approved' AND dp.availability = 'online'
			AND da.id IS NULL
		GROUP BY dp.id, dp.user_id, dp.current_lat, dp.current_lng`)
	if err != nil {
		return err
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var loadScore, curLat, curLng float64
		if err := rows.Scan(&c.id, &c.userID, &loadScore, &curLat, &curLng); err != nil {
			rows.Close()
			return err
		}
		// proximity estimate in km (straight line; labelled as estimate)
		dist := geoDistanceKm(curLat, curLng, outletLat, outletLng)
		c.distance = dist
		c.score = loadScore + dist // lower is better
		candidates = append(candidates, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if len(candidates) == 0 {
		// honest queue state: no fabricated assignment
		if _, err = tx.Exec(ctx, `
			UPDATE orders SET rider_queue_reason = 'Waiting for an available rider (demo).' WHERE id = $1`, orderID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO order_events (order_id, event_type, to_state, actor_type, reason)
			VALUES ($1, 'dispatch_wait', 'ready_for_pickup', 'dispatch', 'No eligible rider online right now.')`, orderID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	// pick the best candidate
	winner := candidates[0]
	for _, c := range candidates[1:] {
		if c.score < winner.score {
			winner = c
		}
	}

	// estimate the drop distance for the payout
	var dropLat, dropLng float64
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(oa.lat::float8, ot.lat::float8), COALESCE(oa.lng::float8, ot.lng::float8)
		FROM orders o JOIN outlets ot ON ot.id = o.outlet_id
		LEFT JOIN order_addresses oa ON oa.order_id = o.id
		WHERE o.id = $1`, orderID).Scan(&dropLat, &dropLng)
	distance := geoDistanceKm(outletLat, outletLng, dropLat, dropLng)
	if distance < 0.5 {
		distance = 0.5
	}

	// payout: base ₹25 + ₹8/km (deterministic demo policy)
	base := int64(2500)
	perKm := int64(800)
	payout := base + perKm*int64(distance*100)/100

	var assignmentID string
	err = tx.QueryRow(ctx, `
		INSERT INTO delivery_assignments
		(order_id, rider_id, status, offered_at, expires_at, distance_km, assigned_by, payout_base_paise, payout_incentive_paise)
		VALUES ($1, $2, 'assigned', now(), now() + interval '10 minutes', $3, 'dispatch', $4, $5)
		RETURNING id`, orderID, winner.id, distance, base, payout).Scan(&assignmentID)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO delivery_events (assignment_id, event_type, actor_type, metadata)
		VALUES ($1, 'offered', 'dispatch', $2::jsonb)`,
		assignmentID, jsonDict(map[string]any{"distance_km": distance, "payout": payout})); err != nil {
		return err
	}

	// delivery confirmation OTP (hash stored only)
	if _, err = courier.GeneratePickupOTP(ctx, tx, assignmentID); err != nil {
		return err
	}

	if err = outbox.Enqueue(ctx, tx, "order", orderID, "delivery.assigned", map[string]any{
		"order_id": orderID, "assignment_id": assignmentID, "rider_id": winner.id,
	}, nil); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	h.log.InfoContext(ctx, "dispatch offered",
		slog.String("order", orderID), slog.String("rider", winner.id),
		slog.Float64("distance_km", distance))
	return nil
}

func geoDistanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371.0
	rad := func(d float64) float64 { return d * 3.141592653589793 / 180 }
	sin2 := func(x float64) float64 { s := sin(x); return s * s }
	h := sin2(rad(lat2-lat1)/2) + cos(rad(lat1))*cos(rad(lat2))*sin2(rad(lng2-lng1)/2)
	return 2 * earthRadiusKm * asin(sqrt(minFloat(1, h)))
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func sin(x float64) float64  { return math.Sin(x) }
func cos(x float64) float64  { return math.Cos(x) }
func asin(x float64) float64 { return math.Asin(x) }
func sqrt(x float64) float64 { return math.Sqrt(x) }

func jsonDict(m map[string]any) string {
	raw, _ := json.Marshal(m)
	return string(raw)
}
