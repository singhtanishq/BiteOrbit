package orders

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/platform/geo"
)

// Track serves GET /orders/{id}/track — owner-only live status with a
// deterministic simulated rider position while the order is on the way.
// No real GPS, no fabricated arrival promises: everything is labelled as a
// demo simulation, derived from assignment timestamps (not live data).
func (h *Handlers) Track(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")

	var status, reference string
	err := h.pool.QueryRow(r.Context(), `
		SELECT status, reference FROM orders WHERE id = $1 AND customer_id = $2`,
		id, p.ID).Scan(&status, &reference)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Order not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	out := map[string]any{
		"order_id":  id,
		"reference": reference,
		"status":    status,
		"simulated": true,
		"demo_note": "Rider movement is a deterministic demo simulation — not a live GPS feed.",
		"polled_at": time.Now().UTC(),
	}

	// assignment state + positions
	var daStatus string
	var daID string
	var outletLat, outletLng, dropLat, dropLng *float64
	err = h.pool.QueryRow(r.Context(), `
		SELECT da.id, da.status, ot.lat::float8, ot.lng::float8, oa.lat::float8, oa.lng::float8
		FROM delivery_assignments da
		JOIN orders o ON o.id = da.order_id
		JOIN outlets ot ON ot.id = o.outlet_id
		LEFT JOIN order_addresses oa ON oa.order_id = o.id
		WHERE da.order_id = $1
		ORDER BY da.created_at DESC LIMIT 1`, id).Scan(&daID, &daStatus, &outletLat, &outletLng, &dropLat, &dropLng)
	if err == nil {
		out["delivery_status"] = daStatus
		if outletLat != nil {
			out["outlet_location"] = geo.Point{Lat: *outletLat, Lng: *outletLng}
		}
		if dropLat != nil {
			out["drop_location"] = geo.Point{Lat: *dropLat, Lng: *dropLng}
		}
		// simulated position between outlet and drop while en route
		if (daStatus == "on_the_way" || daStatus == "picked_up") && outletLat != nil && dropLat != nil {
			var pickedAt, onWayAt time.Time
			_ = h.pool.QueryRow(r.Context(), `
				SELECT COALESCE(o.picked_at, da.accepted_at, now()), COALESCE(o.picked_at, da.accepted_at, now())
				FROM delivery_assignments da JOIN orders o ON o.id = da.order_id
				WHERE da.id = $1`, daID).Scan(&pickedAt, &onWayAt)
			elapsed := time.Since(pickedAt).Minutes()
			etaMin := 12.0
			t := elapsed / etaMin
			if t > 1 {
				t = 1
			}
			pos := geo.SimulateJourney(
				geo.Point{Lat: *outletLat, Lng: *outletLng},
				geo.Point{Lat: *dropLat, Lng: *dropLng}, t)
			out["rider_position"] = pos
			out["rider_position_note"] = "Deterministic demo journey — same inputs produce the same position."
		}
	}

	// rider identity when assigned
	var riderName string
	_ = h.pool.QueryRow(r.Context(), `
		SELECT u.name FROM delivery_assignments da
		JOIN delivery_partners dp ON dp.id = da.rider_id
		JOIN users u ON u.id = dp.user_id
		WHERE da.order_id = $1 AND da.status IN ('accepted','picked_up','on_the_way','delivered')
		ORDER BY da.created_at DESC LIMIT 1`, id).Scan(&riderName)
	if riderName != "" {
		out["rider_name"] = riderName
	}

	// eta range
	var etaMin, etaMax int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COALESCE(eta_min_minutes,30), COALESCE(eta_max_minutes,55) FROM orders WHERE id = $1`, id).
		Scan(&etaMin, &etaMax)
	out["eta_min_minutes"] = etaMin
	out["eta_max_minutes"] = etaMax

	httpx.WriteData(w, r, http.StatusOK, out)
}
