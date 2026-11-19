// Package courier implements the delivery-partner API: availability, offers
// with atomic claiming, the delivery state machine, demo tracking and
// earnings. Riders can never edit their own payout amounts or mark funds
// settled — those are admin/settlement actions.
package courier

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/outbox"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

// riderOf resolves the caller's delivery-partner record (401-grade error if
// the account has no rider profile).
func (h *Handlers) riderOf(ctx context.Context, q db.Querier, userID string) (riderID string, err error) {
	err = q.QueryRow(ctx,
		`SELECT id FROM delivery_partners WHERE user_id = $1`, userID).Scan(&riderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.New(apperr.CodeForbidden, "This account has no delivery-partner profile.")
	}
	return riderID, err
}

type riderProfile struct {
	RiderID      string     `json:"rider_id"`
	Status       string     `json:"status"`
	Availability string     `json:"availability"`
	LastOnline   *time.Time `json:"last_online_at,omitempty"`
	VehicleType  string     `json:"vehicle_type"`
	VehicleNum   string     `json:"vehicle_number"`
	Name         string     `json:"name"`
}

// Profile serves GET /courier/me.
func (h *Handlers) Profile(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var out riderProfile
	err := h.pool.QueryRow(r.Context(), `
		SELECT dp.id, dp.status, dp.availability, dp.last_online_at, dp.vehicle_type,
			COALESCE(dp.vehicle_number,''), u.name
		FROM delivery_partners dp JOIN users u ON u.id = dp.user_id
		WHERE dp.user_id = $1`, p.ID).Scan(
		&out.RiderID, &out.Status, &out.Availability, &out.LastOnline,
		&out.VehicleType, &out.VehicleNum, &out.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden", "This account has no delivery-partner profile.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

type availabilityBody struct {
	Online bool `json:"online"`
}

// SetAvailability serves POST /courier/availability. Approved riders only.
func (h *Handlers) SetAvailability(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body availabilityBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	riderID, err := h.riderOf(r.Context(), h.pool, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	availability := "offline"
	if body.Online {
		availability = "online"
	}
	tag, err := h.pool.Exec(r.Context(), `
		UPDATE delivery_partners SET availability = $2, last_online_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'approved'`, riderID, availability)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden",
			"Only approved delivery partners can go online. Application review is a simulation in the demo.", nil)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"availability": availability})
}

type offerDTO struct {
	AssignmentID     string  `json:"assignment_id"`
	OrderID          string  `json:"order_id"`
	Reference        string  `json:"reference"`
	OutletName       string  `json:"outlet_name"`
	OutletLocality   string  `json:"outlet_locality"`
	DropLocality     string  `json:"drop_locality"`
	DistanceKm       float64 `json:"distance_km"`
	PayoutTotalPaise int64   `json:"payout_total_paise"`
	ExpiresInSec     int     `json:"expires_in_sec"`
	ItemsCount       int     `json:"items_count"`
}

// Offers serves GET /courier/offers — open assignments offered to this rider
// (or unclaimed ones in their base zone). Customer addresses stay hidden
// until acceptance.
func (h *Handlers) Offers(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	riderID, err := h.riderOf(r.Context(), h.pool, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT da.id, da.order_id, o.reference, ot.name, ot.locality,
			COALESCE(oa.locality, '(demo locality)'), COALESCE(da.distance_km, 0),
			da.payout_base_paise + da.payout_incentive_paise,
			EXTRACT(EPOCH FROM (da.expires_at - now()))::int,
			(SELECT COALESCE(SUM(quantity),0) FROM order_items oi WHERE oi.order_id = da.order_id)
		FROM delivery_assignments da
		JOIN orders o ON o.id = da.order_id
		JOIN outlets ot ON ot.id = o.outlet_id
		LEFT JOIN order_addresses oa ON oa.order_id = o.id
		WHERE da.rider_id = $1 AND da.status = 'assigned' AND da.expires_at > now()
		ORDER BY da.offered_at ASC`, riderID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	out := []offerDTO{}
	for rows.Next() {
		var o offerDTO
		if err := rows.Scan(&o.AssignmentID, &o.OrderID, &o.Reference, &o.OutletName,
			&o.OutletLocality, &o.DropLocality, &o.DistanceKm, &o.PayoutTotalPaise,
			&o.ExpiresInSec, &o.ItemsCount); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, o)
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// AcceptOffer serves POST /courier/offers/{id}/accept. The claim is atomic:
// one conditional UPDATE means exactly one rider can win an assignment.
func (h *Handlers) AcceptOffer(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	riderID, err := h.riderOf(r.Context(), h.pool, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	assignmentID := r.PathValue("id")

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	// atomic claim: only from status 'assigned', unexpired, rider must match
	tag, err := tx.Exec(r.Context(), `
		UPDATE delivery_assignments
		SET status = 'accepted', accepted_at = now(), updated_at = now()
		WHERE id = $1 AND rider_id = $2 AND status = 'assigned' AND expires_at > now()`,
		assignmentID, riderID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusConflict, "conflict",
			"This offer expired or was already claimed by another rider.", nil)
		return
	}

	var orderID string
	err = tx.QueryRow(r.Context(), `SELECT order_id FROM delivery_assignments WHERE id = $1`, assignmentID).Scan(&orderID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// order: ready_for_pickup → rider_assigned
	tag, err = tx.Exec(r.Context(), `
		UPDATE orders SET status = 'rider_assigned', updated_at = now()
		WHERE id = $1 AND status = 'ready_for_pickup'`, orderID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			"The order is no longer awaiting a rider.", nil)
		return
	}

	if _, err = tx.Exec(r.Context(), `
		INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id)
		VALUES ($1,'state_changed','ready_for_pickup','rider_assigned','rider',$2)`, orderID, p.ID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO delivery_events (assignment_id, event_type, actor_type, actor_id)
		VALUES ($1,'accepted','rider',$2)`, assignmentID, p.ID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = outbox.Enqueue(r.Context(), tx, "order", orderID, "order.status_changed", map[string]any{
		"order_id": orderID, "from_state": "ready_for_pickup", "to_state": "rider_assigned",
		"reference": "", "customer_id": "",
	}, nil); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"accepted": true, "order_id": orderID})
}

type assignmentDTO struct {
	ID         string     `json:"id"`
	OrderID    string     `json:"order_id"`
	Reference  string     `json:"reference"`
	Status     string     `json:"status"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	Outlet     struct {
		Name     string  `json:"name"`
		Phone    string  `json:"phone"`
		Locality string  `json:"locality"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
	} `json:"outlet"`
	Drop struct {
		Recipient    string  `json:"recipient_name"`
		Phone        string  `json:"phone"`
		Line1        string  `json:"line1"`
		Locality     string  `json:"locality"`
		City         string  `json:"city"`
		Instructions *string `json:"instructions"`
		Lat          float64 `json:"lat"`
		Lng          float64 `json:"lng"`
	} `json:"drop"`
	DistanceKm       float64 `json:"distance_km"`
	PayoutTotalPaise int64   `json:"payout_total_paise"`
	PayoutBreakdown  struct {
		Base      int64 `json:"base_paise"`
		Incentive int64 `json:"incentive_paise"`
	} `json:"payout_breakdown"`
	ItemsCount    int      `json:"items_count"`
	PaymentMethod string   `json:"payment_method"`
	PaymentStatus string   `json:"payment_status"`
	NextStates    []string `json:"allowed_next_states"`
}

// validRiderTransitions maps assignment status → allowed next states.
var validRiderTransitions = map[string][]string{
	"accepted":               {"arriving_at_restaurant", "picked_up", "delivery_failed"},
	"arriving_at_restaurant": {"arrived_at_restaurant", "picked_up", "delivery_failed"},
	"arrived_at_restaurant":  {"picked_up", "delivery_failed"},
	"picked_up":              {"on_the_way", "delivery_failed"},
	"on_the_way":             {"delivered", "delivery_failed"},
}

// Assignment serves GET /courier/assignments/{id} — full pickup/drop details
// once the rider holds the assignment.
func (h *Handlers) Assignment(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	riderID, err := h.riderOf(r.Context(), h.pool, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	id := r.PathValue("id")

	var a assignmentDTO
	var outletLat, outletLng, dropLat, dropLng *float64
	err = h.pool.QueryRow(r.Context(), `
		SELECT da.id, da.order_id, o.reference, da.status, da.accepted_at,
			ot.name, ot.phone, ot.locality, ot.lat::float8, ot.lng::float8,
			oa.recipient_name, oa.recipient_phone, oa.line1, oa.locality, oa.city,
			oa.instructions, oa.lat::float8, oa.lng::float8,
			COALESCE(da.distance_km,0), da.payout_base_paise, da.payout_incentive_paise,
			pm.method, pm.status
		FROM delivery_assignments da
		JOIN orders o ON o.id = da.order_id
		JOIN outlets ot ON ot.id = o.outlet_id
		JOIN order_addresses oa ON oa.order_id = o.id
		LEFT JOIN payments pm ON pm.order_id = o.id
		WHERE da.id = $1 AND da.rider_id = $2`, id, riderID).Scan(
		&a.ID, &a.OrderID, &a.Reference, &a.Status, &a.AcceptedAt,
		&a.Outlet.Name, &a.Outlet.Phone, &a.Outlet.Locality, &outletLat, &outletLng,
		&a.Drop.Recipient, &a.Drop.Phone, &a.Drop.Line1, &a.Drop.Locality, &a.Drop.City,
		&a.Drop.Instructions, &dropLat, &dropLng,
		&a.DistanceKm, &a.PayoutBreakdown.Base, &a.PayoutBreakdown.Incentive,
		&a.PaymentMethod, &a.PaymentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Assignment not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if outletLat != nil {
		a.Outlet.Lat = *outletLat
		a.Outlet.Lng = *outletLng
	}
	if dropLat != nil {
		a.Drop.Lat = *dropLat
		a.Drop.Lng = *dropLng
	}
	a.PayoutTotalPaise = a.PayoutBreakdown.Base + a.PayoutBreakdown.Incentive
	a.NextStates = validRiderTransitions[a.Status]
	if a.NextStates == nil {
		a.NextStates = []string{}
	}

	var items int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(SUM(quantity),0) FROM order_items WHERE order_id = $1`, a.OrderID).Scan(&items)
	a.ItemsCount = items

	httpx.WriteData(w, r, http.StatusOK, a)
}

type stateBody struct {
	State  string `json:"state"`
	OTP    string `json:"otp,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// UpdateAssignmentState serves POST /courier/assignments/{id}/state.
// Transitions are validated against the state machine and recorded with the
// rider's identity. 'delivered' requires the OTP when one exists.
func (h *Handlers) UpdateAssignmentState(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	riderID, err := h.riderOf(r.Context(), h.pool, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	assignmentID := r.PathValue("id")
	var body stateBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var current, orderID string
	err = tx.QueryRow(r.Context(), `
		SELECT status, order_id FROM delivery_assignments
		WHERE id = $1 AND rider_id = $2 FOR UPDATE`, assignmentID, riderID).Scan(&current, &orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Assignment not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	allowed := validRiderTransitions[current]
	ok := false
	for _, s := range allowed {
		if s == body.State {
			ok = true
			break
		}
	}
	if !ok {
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			fmt.Sprintf("Cannot move from %s to %s.", current, body.State), nil)
		return
	}

	if body.State == "delivered" && body.Reason == "" {
		// OTP verification (hash-only storage)
		var otpHash *string
		_ = tx.QueryRow(r.Context(),
			`SELECT pickup_otp_hash FROM delivery_assignments WHERE id = $1`, assignmentID).Scan(&otpHash)
		if otpHash != nil && *otpHash != "" {
			if verifyOTP(body.OTP, *otpHash) != true {
				httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
					"Invalid delivery confirmation code.", nil)
				return
			}
		}
	}

	// persist assignment transition
	tag, err := tx.Exec(r.Context(), `
		UPDATE delivery_assignments SET status = $2, updated_at = now()
		WHERE id = $1 AND status = $3`, assignmentID, body.State, current)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			"Assignment changed concurrently. Refresh and retry.", nil)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO delivery_events (assignment_id, event_type, actor_type, actor_id, metadata)
		VALUES ($1,$2,'rider',$3,$4::jsonb)`,
		assignmentID, body.State, p.ID, mustJSON(map[string]any{"reason": body.Reason})); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// map to the order status machine
	orderTo := ""
	switch body.State {
	case "picked_up":
		orderTo = "picked_up"
	case "on_the_way":
		orderTo = "on_the_way"
	case "delivered":
		orderTo = "delivered"
	case "delivery_failed":
		orderTo = "delivery_failed"
	}

	if orderTo != "" {
		var prevStatus string
		_ = tx.QueryRow(r.Context(), `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&prevStatus)
		tag, err = tx.Exec(r.Context(), `
			UPDATE orders SET status = $2, updated_at = now(),
				picked_at = CASE WHEN $2 = 'picked_up' THEN now() ELSE picked_at END,
				delivered_at = CASE WHEN $2 = 'delivered' THEN now() ELSE delivered_at END,
				closed_at = CASE WHEN $2 IN ('delivered','delivery_failed') THEN now() ELSE closed_at END
			WHERE id = $1 AND status = $3`, orderID, orderTo, prevStatus)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if tag.RowsAffected() == 0 {
			httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
				"Order state changed concurrently. Refresh and retry.", nil)
			return
		}
		if _, err = tx.Exec(r.Context(), `
			INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id, reason)
			VALUES ($1,'state_changed',$2,$3,'rider',$4,NULLIF($5,''))`,
			orderID, prevStatus, orderTo, p.ID, body.Reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}

	// delivery-side effects
	switch body.State {
	case "delivered":
		// COD: mark collected
		if _, err = tx.Exec(r.Context(), `
			UPDATE payments SET status='cod_collected', captured_at=now(), updated_at=now()
			WHERE order_id=$1 AND method='cod' AND status='cod_due'`, orderID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		// rider earnings (base + incentive + tip from the order)
		if _, err = tx.Exec(r.Context(), `
			INSERT INTO rider_earnings (rider_id, assignment_id, base_pay_paise, incentive_paise, tip_paise, total_paise, status)
			SELECT da.rider_id, da.id, da.payout_base_paise, da.payout_incentive_paise,
				COALESCE(o.tip_paise, 0),
				da.payout_base_paise + da.payout_incentive_paise + COALESCE(o.tip_paise, 0),
				'payable'
			FROM delivery_assignments da JOIN orders o ON o.id = da.order_id
			WHERE da.id = $1
			ON CONFLICT (assignment_id) DO NOTHING`, assignmentID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		// refund nothing; clear queued reasons
		if _, err = tx.Exec(r.Context(),
			`UPDATE orders SET rider_queue_reason = NULL WHERE id = $1`, orderID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	case "delivery_failed":
		// captured online payments refunded; COD due cancelled
		if _, err = tx.Exec(r.Context(), `
			UPDATE payments SET status='refunded', updated_at=now()
			WHERE order_id=$1 AND method <> 'cod' AND status='captured'`, orderID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `
			INSERT INTO refunds (payment_id, order_id, amount_paise, status, reason, provider_reference, initiated_by, completed_at)
			SELECT pm.id, pm.order_id, pm.amount_paise, 'processed', $2, pm.provider_reference || '-fail', 'system', now()
			FROM payments pm WHERE pm.order_id = $1 AND pm.method <> 'cod' AND pm.status='refunded'`,
			orderID, "Delivery failed: "+body.Reason); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `
			UPDATE payments SET status='cancelled', updated_at=now()
			WHERE order_id=$1 AND method='cod' AND status='cod_due'`, orderID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}

	if err = outbox.Enqueue(r.Context(), tx, "order", orderID, "order.status_changed", map[string]any{
		"order_id": orderID, "from_state": "", "to_state": orderTo,
		"reason": body.Reason, "reference": "", "customer_id": "",
	}, nil); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"assignment_status": body.State, "order_status": orderTo})
}

// verifyOTP compares a plaintext OTP with the stored hash (constant time).
func verifyOTP(otp, storedHex string) bool {
	sum := sha256.Sum256([]byte(otp))
	got := hex.EncodeToString(sum[:])
	stored := storedHex
	// stored format: sha256 hex
	if len(stored) != 64 {
		return false
	}
	return subtleEqual(got, stored)
}

func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func mustJSON(m map[string]any) string {
	raw, _ := jsonMarshal(m)
	return string(raw)
}

// GeneratePickupOTP creates a 4-digit code and stores only its hash.
func GeneratePickupOTP(ctx context.Context, q db.Querier, assignmentID string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return "", err
	}
	otp := fmt.Sprintf("%04d", n.Int64())
	sum := sha256.Sum256([]byte(otp))
	if _, err = q.Exec(ctx,
		`UPDATE delivery_assignments SET pickup_otp_hash = $2 WHERE id = $1`,
		assignmentID, hex.EncodeToString(sum[:])); err != nil {
		return "", err
	}
	return otp, nil
}
