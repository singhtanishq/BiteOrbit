// Package portal implements the restaurant partner API: outlet scoping,
// order fulfilment transitions, menu management and outlet analytics.
// Every outlet access is authorised through outlet_staff / organisation
// ownership — record-level checks live in the queries.
package portal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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

// accessibleOutlet authorises the caller for an outlet: owners reach any
// outlet of their organisation, managers/staff only explicitly granted ones.
// Returns the outlet id on success; a 404-equivalent error otherwise.
func (h *Handlers) accessibleOutlet(ctx context.Context, q db.Querier, userID, outletID string) error {
	var n int
	err := q.QueryRow(ctx, `
		SELECT COUNT(*) FROM outlets o
		JOIN restaurant_organisations org ON org.id = o.organisation_id
		LEFT JOIN outlet_staff os ON os.outlet_id = o.id AND os.user_id = $1
		LEFT JOIN user_roles ur ON ur.user_id = $1 AND ur.role IN ('restaurant_owner','restaurant_manager','restaurant_staff')
		WHERE o.id = $2 AND o.deleted_at IS NULL
		  AND (org.owner_user_id = $1 OR os.user_id IS NOT NULL)
		  AND ur.user_id IS NOT NULL`, userID, outletID).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.New(apperr.CodeNotFound, "Outlet not found or not accessible with your account.")
	}
	return nil
}

// MyOutlets serves GET /restaurant-portal/outlets.
func (h *Handlers) MyOutlets(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT DISTINCT o.id, o.name, o.locality, o.status, o.min_order_paise, o.packaging_fee_paise,
			o.prep_minutes_min, o.prep_minutes_max, os.role
		FROM outlets o
		JOIN restaurant_organisations org ON org.id = o.organisation_id
		LEFT JOIN outlet_staff os ON os.outlet_id = o.id AND os.user_id = $1
		WHERE o.deleted_at IS NULL AND (org.owner_user_id = $1 OR os.user_id = $1)
		ORDER BY o.name`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type outlet struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Locality  string `json:"locality"`
		Status    string `json:"status"`
		MinOrder  int64  `json:"min_order_paise"`
		Packaging int64  `json:"packaging_fee_paise"`
		PrepMin   int    `json:"prep_minutes_min"`
		PrepMax   int    `json:"prep_minutes_max"`
		Relation  string `json:"relation"` // owner | manager | staff
	}
	out := []outlet{}
	for rows.Next() {
		var o outlet
		if err := rows.Scan(&o.ID, &o.Name, &o.Locality, &o.Status, &o.MinOrder, &o.Packaging,
			&o.PrepMin, &o.PrepMax, &o.Relation); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, o)
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// ListOutletOrders serves GET /restaurant-portal/orders?outlet_id=&status=.
func (h *Handlers) ListOutletOrders(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	outletID := r.URL.Query().Get("outlet_id")
	if outletID == "" {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "outlet_id is required.", nil)
		return
	}
	if err := h.accessibleOutlet(r.Context(), h.pool, p.ID, outletID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	statusFilter := r.URL.Query().Get("status")
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	limit := atoiDefault(r.URL.Query().Get("limit"), 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	where := "o.outlet_id = $1"
	args := []any{outletID}
	if statusFilter != "" {
		args = append(args, statusFilter)
		where += fmt.Sprintf(" AND o.status = $%d", len(args))
	}
	args = append(args, limit, (page-1)*limit)

	rows, err := h.pool.Query(r.Context(), fmt.Sprintf(`
		SELECT o.id, o.reference, o.status, o.placed_at, o.grand_total_paise,
			o.items_subtotal_paise, o.instructions,
			COALESCE(json_agg(json_build_object('name', oi.name_snapshot, 'qty', oi.quantity, 'unit', oi.unit_price_paise, 'instructions', oi.special_instructions) ORDER BY oi.id) FILTER (WHERE oi.id IS NOT NULL), '[]'),
			COALESCE((SELECT status FROM delivery_assignments da WHERE da.order_id = o.id ORDER BY da.created_at DESC LIMIT 1), '')
		FROM orders o
		LEFT JOIN order_items oi ON oi.order_id = o.id
		WHERE %s
		GROUP BY o.id
		ORDER BY o.placed_at DESC
		LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type item struct {
		Name         string  `json:"name"`
		Qty          int     `json:"qty"`
		Unit         int64   `json:"unit_price_paise"`
		Instructions *string `json:"instructions"`
	}
	type orderRow struct {
		ID           string    `json:"id"`
		Reference    string    `json:"reference"`
		Status       string    `json:"status"`
		PlacedAt     time.Time `json:"placed_at"`
		GrandTotal   int64     `json:"grand_total_paise"`
		ItemsTotal   int64     `json:"items_subtotal_paise"`
		Instructions *string   `json:"instructions"`
		Items        []item    `json:"items"`
		Delivery     string    `json:"delivery_status"`
	}
	out := []orderRow{}
	for rows.Next() {
		var o orderRow
		if err := rows.Scan(&o.ID, &o.Reference, &o.Status, &o.PlacedAt, &o.GrandTotal,
			&o.ItemsTotal, &o.Instructions, &o.Items, &o.Delivery); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, o)
	}
	httpx.WriteMeta(w, r, out, map[string]any{"page": page, "limit": limit})
}

// transitionGuard validates restaurant-side transitions:
//
//	placed → restaurant_confirmed (accept)
//	restaurant_confirmed → preparing
//	preparing → ready_for_pickup
//
// Restaurants can reject fresh orders. They can never mark delivered.
func (h *Handlers) transition(ctx context.Context, q db.Querier, orderID, from, to, actorID, reason string) error {
	tag, err := q.Exec(ctx, `
		UPDATE orders SET status = $3, updated_at = now(),
			confirmed_at = CASE WHEN $3 = 'restaurant_confirmed' THEN now() ELSE confirmed_at END,
			preparing_at = CASE WHEN $3 = 'preparing' THEN now() ELSE preparing_at END,
			ready_at = CASE WHEN $3 = 'ready_for_pickup' THEN now() ELSE ready_at END,
			closed_at = CASE WHEN $3 = 'rejected' THEN now() ELSE closed_at END
		WHERE id = $1 AND status = $2 AND outlet_id IN (SELECT outlet_id FROM orders WHERE id = $1)`,
		orderID, from, to)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.New(apperr.CodeInvalidState,
			fmt.Sprintf("Order is not in %s state anymore.", from))
	}
	_, err = q.Exec(ctx, `
		INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id, reason)
		VALUES ($1, 'state_changed', $2, $3, 'restaurant', $4, NULLIF($5,''))`,
		orderID, from, to, actorID, reason)
	if err != nil {
		return err
	}
	return outbox.Enqueue(ctx, q, "order", orderID, "order.status_changed", map[string]any{
		"order_id": orderID, "from_state": from, "to_state": to, "reason": reason,
		"reference": "", "customer_id": "",
	}, nil)
}

// orderOutlet authorises AND returns the order's outlet for this caller.
func (h *Handlers) orderOutlet(ctx context.Context, q db.Querier, userID, orderID string) (string, error) {
	var outletID string
	err := q.QueryRow(ctx, `
		SELECT o.outlet_id FROM orders o
		JOIN outlets ot ON ot.id = o.outlet_id
		JOIN restaurant_organisations org ON org.id = ot.organisation_id
		LEFT JOIN outlet_staff os ON os.outlet_id = ot.id AND os.user_id = $1
		LEFT JOIN user_roles ur ON ur.user_id = $1 AND ur.role IN ('restaurant_owner','restaurant_manager','restaurant_staff')
		WHERE o.id = $2 AND (org.owner_user_id = $1 OR os.user_id IS NOT NULL) AND ur.user_id IS NOT NULL`,
		userID, orderID).Scan(&outletID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.New(apperr.CodeNotFound, "Order not found or not accessible with your account.")
	}
	if err != nil {
		return "", err
	}
	return outletID, nil
}

type actionBody struct {
	Reason              string `json:"reason"`
	PrepEstimateMinutes int    `json:"prep_estimate_minutes"`
}

// Accept serves POST /restaurant-portal/orders/{id}/accept.
func (h *Handlers) Accept(w http.ResponseWriter, r *http.Request) {
	h.doTransition(w, r, "placed", "restaurant_confirmed", false)
}

// Reject serves POST /restaurant-portal/orders/{id}/reject.
func (h *Handlers) Reject(w http.ResponseWriter, r *http.Request) {
	h.doTransition(w, r, "placed", "rejected", true)
}

// StartPrepare serves POST /restaurant-portal/orders/{id}/prepare.
func (h *Handlers) StartPrepare(w http.ResponseWriter, r *http.Request) {
	h.doTransition(w, r, "restaurant_confirmed", "preparing", false)
}

// MarkReady serves POST /restaurant-portal/orders/{id}/ready.
func (h *Handlers) MarkReady(w http.ResponseWriter, r *http.Request) {
	h.doTransition(w, r, "preparing", "ready_for_pickup", false)
}

func (h *Handlers) doTransition(w http.ResponseWriter, r *http.Request, from, to string, needsReason bool) {
	p := httpx.UserFrom(r.Context())
	orderID := r.PathValue("id")
	var body actionBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if needsReason && len(body.Reason) < 4 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"A rejection reason is required (min 4 characters).", nil)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	// authorise + ensure the order belongs to an accessible outlet and is in
	// the expected source state (prevents double-accept races)
	var outletID string
	err = tx.QueryRow(r.Context(), `
		SELECT ot.id FROM orders o
		JOIN outlets ot ON ot.id = o.outlet_id
		JOIN restaurant_organisations org ON org.id = ot.organisation_id
		LEFT JOIN outlet_staff os ON os.outlet_id = ot.id AND os.user_id = $1
		LEFT JOIN user_roles ur ON ur.user_id = $1 AND ur.role IN ('restaurant_owner','restaurant_manager','restaurant_staff')
		WHERE o.id = $2 AND o.status = $3 AND (org.owner_user_id = $1 OR os.user_id IS NOT NULL) AND ur.user_id IS NOT NULL
		FOR UPDATE OF o`, p.ID, orderID, from).Scan(&outletID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			fmt.Sprintf("Order is not in %s state anymore or is not accessible.", from), nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err = h.transition(r.Context(), tx, orderID, from, to, p.ID, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// rejections refund captured online payments via the payments package
	// (handled by the worker for offline consistency; keep record here)
	if to == "rejected" {
		if _, err = tx.Exec(r.Context(), `
			UPDATE payments SET status = 'refunded', updated_at = now()
			WHERE order_id = $1 AND method <> 'cod' AND status = 'captured'`, orderID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `
			INSERT INTO refunds (payment_id, order_id, amount_paise, status, reason, provider_reference, initiated_by, completed_at)
			SELECT pm.id, pm.order_id, pm.amount_paise, 'processed', $2, pm.provider_reference || '-rej', 'system', now()
			FROM payments pm WHERE pm.order_id = $1 AND pm.method <> 'cod' AND pm.status = 'refunded'`, orderID, body.Reason); err != nil {
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

	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"status": to, "outlet_id": outletID})
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
