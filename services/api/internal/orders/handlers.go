// Package orders serves the customer-facing order views and the customer
// cancellation flow. All lifecycle transitions are guarded by the current
// database state — no client can push an order into an invalid state.
package orders

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/outbox"
	"biteorbit/services/api/internal/platform/payments"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

// List serves GET /orders — the caller's own orders, newest first.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	limit := parseIntDefault(r.URL.Query().Get("limit"), 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	offset := (page - 1) * limit

	rows, err := h.pool.Query(r.Context(), `
		SELECT o.id, o.reference, o.status, o.grand_total_paise, o.placed_at,
			o.eta_min_minutes, o.eta_max_minutes, o.currency, ot.name,
			COALESCE((SELECT status FROM payments pm WHERE pm.order_id = o.id ORDER BY pm.created_at DESC LIMIT 1), 'none')
		FROM orders o JOIN outlets ot ON ot.id = o.outlet_id
		WHERE o.customer_id = $1
		ORDER BY o.created_at DESC
		LIMIT $2 OFFSET $3`, p.ID, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type orderRow struct {
		ID            string    `json:"id"`
		Reference     string    `json:"reference"`
		Status        string    `json:"status"`
		GrandTotal    int64     `json:"grand_total_paise"`
		PlacedAt      time.Time `json:"placed_at"`
		EtaMin        int       `json:"eta_min_minutes"`
		EtaMax        int       `json:"eta_max_minutes"`
		Currency      string    `json:"currency"`
		OutletName    string    `json:"outlet_name"`
		PaymentStatus string    `json:"payment_status"`
	}
	out := []orderRow{}
	for rows.Next() {
		var o orderRow
		if err := rows.Scan(&o.ID, &o.Reference, &o.Status, &o.GrandTotal, &o.PlacedAt,
			&o.EtaMin, &o.EtaMax, &o.Currency, &o.OutletName, &o.PaymentStatus); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, o)
	}
	if rows.Err() != nil {
		httpx.WriteError(w, r, rows.Err())
		return
	}
	var total int
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM orders WHERE customer_id = $1`, p.ID).Scan(&total); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteMeta(w, r, out, map[string]any{"page": page, "limit": limit, "total": total})
}

// Get serves GET /orders/{id} — owner-only detail with the full timeline.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")

	var o struct {
		ID           string    `json:"id"`
		Reference    string    `json:"reference"`
		Status       string    `json:"status"`
		PlacedAt     time.Time `json:"placed_at"`
		GrandTotal   int64     `json:"grand_total_paise"`
		ItemsSub     int64     `json:"items_subtotal_paise"`
		CouponDisc   int64     `json:"coupon_discount_paise"`
		Packaging    int64     `json:"packaging_fee_paise"`
		Delivery     int64     `json:"delivery_fee_paise"`
		Platform     int64     `json:"platform_fee_paise"`
		SmallOrder   int64     `json:"small_order_fee_paise"`
		Surge        int64     `json:"surge_fee_paise"`
		Tip          int64     `json:"tip_paise"`
		Tax          int64     `json:"tax_paise"`
		Instructions *string   `json:"instructions"`
		ETAmin       int       `json:"eta_min_minutes"`
		ETAMax       int       `json:"eta_max_minutes"`
		OutletID     string    `json:"outlet_id"`
		OutletName   string    `json:"outlet_name"`
		OutletPhone  string    `json:"outlet_phone"`
		PricingRule  int       `json:"pricing_rule_version"`
	}
	err := h.pool.QueryRow(r.Context(), `
		SELECT o.id, o.reference, o.status, o.placed_at, o.grand_total_paise,
			o.items_subtotal_paise, o.coupon_discount_paise, o.packaging_fee_paise,
			o.delivery_fee_paise, o.platform_fee_paise, o.small_order_fee_paise,
			o.surge_fee_paise, o.tip_paise, o.tax_paise, o.instructions,
			o.eta_min_minutes, o.eta_max_minutes, o.outlet_id, ot.name, ot.phone,
			COALESCE((SELECT version FROM pricing_rules pr WHERE pr.id = o.pricing_rule_id), 0)
		FROM orders o JOIN outlets ot ON ot.id = o.outlet_id
		WHERE o.id = $1 AND o.customer_id = $2`, id, p.ID).Scan(
		&o.ID, &o.Reference, &o.Status, &o.PlacedAt, &o.GrandTotal,
		&o.ItemsSub, &o.CouponDisc, &o.Packaging, &o.Delivery, &o.Platform,
		&o.SmallOrder, &o.Surge, &o.Tip, &o.Tax, &o.Instructions,
		&o.ETAmin, &o.ETAMax, &o.OutletID, &o.OutletName, &o.OutletPhone, &o.PricingRule)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Order not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// items with modifiers
	type modDTO struct {
		Group  string `json:"group"`
		Option string `json:"option"`
		Delta  int64  `json:"price_delta_paise"`
	}
	type itemDTO struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		IsVeg        bool     `json:"is_veg"`
		Quantity     int      `json:"quantity"`
		UnitPaise    int64    `json:"unit_price_paise"`
		TotalPaise   int64    `json:"item_total_paise"`
		Instructions *string  `json:"special_instructions"`
		Modifiers    []modDTO `json:"modifiers"`
	}
	items := []itemDTO{}
	irows, err := h.pool.Query(r.Context(), `
		SELECT oi.id, oi.name_snapshot, oi.is_veg_snapshot, oi.quantity, oi.unit_price_paise,
			oi.item_total_paise, oi.special_instructions
		FROM order_items oi WHERE oi.order_id = $1 ORDER BY oi.id`, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	itemIDs := []string{}
	for irows.Next() {
		var it itemDTO
		if err := irows.Scan(&it.ID, &it.Name, &it.IsVeg, &it.Quantity, &it.UnitPaise,
			&it.TotalPaise, &it.Instructions); err != nil {
			irows.Close()
			httpx.WriteError(w, r, err)
			return
		}
		it.Modifiers = []modDTO{}
		items = append(items, it)
		itemIDs = append(itemIDs, it.ID)
	}
	irows.Close()
	if len(itemIDs) > 0 {
		mrows, err := h.pool.Query(r.Context(), `
			SELECT order_item_id, group_name_snapshot, option_name_snapshot, price_delta_paise
			FROM order_item_modifiers WHERE order_item_id = ANY($1)`, itemIDs)
		if err == nil {
			for mrows.Next() {
				var m struct {
					ItemID string
					Mod    modDTO
				}
				if mrows.Scan(&m.ItemID, &m.Mod.Group, &m.Mod.Option, &m.Mod.Delta) == nil {
					for i := range items {
						if items[i].ID == m.ItemID {
							items[i].Modifiers = append(items[i].Modifiers, m.Mod)
						}
					}
				}
			}
			mrows.Close()
		}
	}

	// events timeline
	type eventDTO struct {
		ID        int64     `json:"id"`
		Type      string    `json:"event_type"`
		From      *string   `json:"from_state"`
		To        *string   `json:"to_state"`
		Actor     string    `json:"actor_type"`
		Reason    *string   `json:"reason"`
		CreatedAt time.Time `json:"created_at"`
	}
	events := []eventDTO{}
	erows, err := h.pool.Query(r.Context(), `
		SELECT id, event_type, from_state, to_state, actor_type, reason, created_at
		FROM order_events WHERE order_id = $1 ORDER BY id`, id)
	if err == nil {
		for erows.Next() {
			var e eventDTO
			if erows.Scan(&e.ID, &e.Type, &e.From, &e.To, &e.Actor, &e.Reason, &e.CreatedAt) == nil {
				events = append(events, e)
			}
		}
		erows.Close()
	}

	// payment summary
	type paymentDTO struct {
		ID        string  `json:"id"`
		Method    string  `json:"method"`
		Status    string  `json:"status"`
		Amount    int64   `json:"amount_paise"`
		Failure   *string `json:"failure_reason,omitempty"`
		Simulated bool    `json:"simulated"`
	}
	payment := paymentDTO{Simulated: true}
	var hasPayment bool
	prow := h.pool.QueryRow(r.Context(), `
		SELECT id, method, status, amount_paise, failure_reason
		FROM payments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`, id)
	if err := prow.Scan(&payment.ID, &payment.Method, &payment.Status, &payment.Amount, &payment.Failure); err == nil {
		hasPayment = true
	}

	// refunds
	type refundDTO struct {
		ID        string    `json:"id"`
		Amount    int64     `json:"amount_paise"`
		Status    string    `json:"status"`
		Reason    string    `json:"reason"`
		CreatedAt time.Time `json:"created_at"`
	}
	refunds := []refundDTO{}
	rrows, err := h.pool.Query(r.Context(), `
		SELECT id, amount_paise, status, reason, created_at
		FROM refunds WHERE order_id = $1 ORDER BY created_at DESC`, id)
	if err == nil {
		for rrows.Next() {
			var rf refundDTO
			if rrows.Scan(&rf.ID, &rf.Amount, &rf.Status, &rf.Reason, &rf.CreatedAt) == nil {
				refunds = append(refunds, rf)
			}
		}
		rrows.Close()
	}

	// delivery address summary (privacy-appropriate: no coordinates to client)
	var addrSummary string
	_ = h.pool.QueryRow(r.Context(), `
		SELECT recipient_name || ' · ' || line1 || ', ' || locality || ' ' || postal_code
		FROM order_addresses WHERE order_id = $1`, id).Scan(&addrSummary)

	// rider info only once assigned
	riderName := ""
	var riderPhone *string
	_ = h.pool.QueryRow(r.Context(), `
		SELECT u.name, u.phone
		FROM delivery_assignments da JOIN delivery_partners dp ON dp.id = da.rider_id
		JOIN users u ON u.id = dp.user_id
		WHERE da.order_id = $1 AND da.status IN ('accepted','arrived_at_restaurant','arrived_at_restaurant','picked_up','on_the_way','delivered')
		ORDER BY da.created_at DESC LIMIT 1`, id).Scan(&riderName, &riderPhone)

	canCancel := h.canCancel(r.Context(), id)

	httpx.WriteData(w, r, http.StatusOK, map[string]any{
		"id": o.ID, "reference": o.Reference, "status": o.Status,
		"placed_at": o.PlacedAt, "eta_min_minutes": o.ETAmin, "eta_max_minutes": o.ETAMax,
		"outlet": map[string]any{"id": o.OutletID, "name": o.OutletName, "phone": o.OutletPhone},
		"breakdown": map[string]any{
			"items_subtotal_paise": o.ItemsSub, "coupon_discount_paise": o.CouponDisc,
			"packaging_fee_paise": o.Packaging, "delivery_fee_paise": o.Delivery,
			"platform_fee_paise": o.Platform, "small_order_fee_paise": o.SmallOrder,
			"surge_fee_paise": o.Surge, "tip_paise": o.Tip, "tax_paise": o.Tax,
			"grand_total_paise": o.GrandTotal, "pricing_rule_version": o.PricingRule,
		},
		"items": items, "events": events,
		"payment": map[string]any{
			"present": hasPayment, "payment": payment,
		},
		"refunds":      refunds,
		"address":      addrSummary,
		"rider":        map[string]any{"name": riderName, "phone": riderPhone},
		"instructions": o.Instructions,
		"can_cancel":   canCancel,
		"demo_note":    "Rider position and timings are simulated in the local demo.",
	})
}

// canCancel implements the customer cancellation policy: allowed until the
// food is picked up (placed/confirmed/preparing/ready) — after pickup the
// customer must use support.
func (h *Handlers) canCancel(ctx context.Context, orderID string) bool {
	var status string
	if err := h.pool.QueryRow(ctx,
		`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status); err != nil {
		return false
	}
	switch status {
	case "placed", "restaurant_confirmed", "preparing", "ready_for_pickup":
		return true
	}
	return false
}

type cancelBody struct {
	Reason string `json:"reason"`
}

// Cancel applies the cancellation policy, refunds captured online payments
// (simulated locally) and appends the event trail. State-guarded: two racing
// cancels produce one cancellation.
func (h *Handlers) Cancel(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")
	var body cancelBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(body.Reason) < 4 || len(body.Reason) > 300 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Please give a short cancellation reason (4–300 characters).", nil)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	// state guard: only these statuses are customer-cancellable
	var fromStatus string
	err = tx.QueryRow(r.Context(), `
		SELECT status FROM orders WHERE id = $1 AND customer_id = $2
		FOR UPDATE`, id, p.ID).Scan(&fromStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		// owner-scoped: a foreign order is indistinguishable from a missing one
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Order not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	switch fromStatus {
	case "placed", "restaurant_confirmed", "preparing", "ready_for_pickup":
		// cancellable
	default:
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			"This order can no longer be cancelled here. Contact support for help.", nil)
		return
	}

	if _, err = tx.Exec(r.Context(), `
		UPDATE orders SET status='cancel_requested', cancel_reason=$2, cancelled_by_actor='customer', updated_at=now()
		WHERE id=$1`, id, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id, reason)
		VALUES ($1,'state_changed',$2,'cancel_requested','customer',$3,$4)`, id, fromStatus, p.ID, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// refund handling for captured online payments (mock: instant)
	var method, payStatus, payID string
	var provRef *string
	var amount int64
	err = tx.QueryRow(r.Context(), `
		SELECT method, status, id, provider_reference, amount_paise
		FROM payments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`, id).Scan(
		&method, &payStatus, &payID, &provRef, &amount)
	refunded := false
	if err == nil && method != "cod" && payStatus == "captured" {
		providerRef, rerr := payments.NewMock().Refund(r.Context(), deref(provRef), amount, body.Reason)
		if rerr == nil {
			if _, err = tx.Exec(r.Context(), `
				INSERT INTO refunds (payment_id, order_id, amount_paise, status, reason, provider_reference, initiated_by, completed_at)
				VALUES ($1,$2,$3,'processed',$4,$5,'customer',now())`,
				payID, id, amount, body.Reason, providerRef); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			if _, err = tx.Exec(r.Context(),
				`UPDATE payments SET status='refunded', updated_at=now() WHERE id=$1`, payID); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			refunded = true
		}
	}

	// finalise cancellation
	if _, err = tx.Exec(r.Context(), `
		UPDATE orders SET status='cancelled', closed_at=now(), updated_at=now() WHERE id=$1
		AND status='cancel_requested'`, id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id, reason)
		VALUES ($1,'state_changed','cancel_requested','cancelled','system',NULL,'Cancellation approved within policy window')`, id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = outbox.Enqueue(r.Context(), tx, "order", id, "order.cancelled", map[string]any{
		"order_id": id, "customer_id": p.ID, "reason": body.Reason,
		"from_state": fromStatus, "to_state": "cancelled", "refund": refunded,
	}, ptr(fmt.Sprintf("order-status:%s:cancelled", id))); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// customer-visible status notification shares the generic worker path
	if err = outbox.Enqueue(r.Context(), tx, "order", id, "order.status_changed", map[string]any{
		"order_id": id, "customer_id": p.ID, "reference": "",
		"from_state": fromStatus, "to_state": "cancelled", "reason": body.Reason,
	}, nil); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{
		"status": "cancelled", "refund_initiated": refunded,
		"simulated_refund": refunded,
	})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptr(s string) *string { return &s }

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
