// Customer retention features: favourites and reorder (P1). Reorder rebuilds
// a cart from a past order at CURRENT menu prices and availability — never
// reusing historical prices (see ADR 006).
package orders

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/httpx"
)

// --- favourites ------------------------------------------------------------

// ListFavourites serves GET /me/favourites.
func (h *Handlers) ListFavourites(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT o.id, o.name, o.locality, o.city, o.price_band, o.is_pure_veg,
			COALESCE((SELECT ROUND(AVG(rv.rating)::numeric, 2)::float8 FROM reviews rv
				WHERE rv.outlet_id = o.id AND rv.status = 'published'), 0),
			EXISTS(SELECT 1 FROM outlet_hours oh WHERE oh.outlet_id = o.id)
		FROM favourites f JOIN outlets o ON o.id = f.outlet_id
		WHERE f.user_id = $1 AND o.deleted_at IS NULL
		ORDER BY f.created_at DESC`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type fav struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		Locality  string  `json:"locality"`
		City      string  `json:"city"`
		PriceBand int     `json:"price_band"`
		IsPureVeg bool    `json:"is_pure_veg"`
		Rating    float64 `json:"rating"`
		HasHours  bool    `json:"has_hours"`
	}
	out := []fav{}
	for rows.Next() {
		var v fav
		if rows.Scan(&v.ID, &v.Name, &v.Locality, &v.City, &v.PriceBand, &v.IsPureVeg, &v.Rating, &v.HasHours) == nil {
			out = append(out, v)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// AddFavourite serves POST /me/favourites/{outlet_id}.
func (h *Handlers) AddFavourite(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	outletID := r.PathValue("outlet_id")
	var exists bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM outlets WHERE id = $1 AND deleted_at IS NULL)`, outletID).Scan(&exists); err != nil || !exists {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Restaurant not found.", nil)
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO favourites (user_id, outlet_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, p.ID, outletID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{"favourited": true})
}

// RemoveFavourite serves DELETE /me/favourites/{outlet_id}.
func (h *Handlers) RemoveFavourite(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	outletID := r.PathValue("outlet_id")
	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM favourites WHERE user_id = $1 AND outlet_id = $2`, p.ID, outletID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Not in favourites.", nil)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"removed": true})
}

// --- reorder ---------------------------------------------------------------

type reorderResult struct {
	CartID     string `json:"cart_id"`
	OutletID   string `json:"outlet_id"`
	Added      int    `json:"added"`
	Skipped    int    `json:"skipped"`
	SkippedWhy string `json:"skipped_reason,omitempty"`
	Replaced   bool   `json:"replaced_other_outlet_cart"`
}

// Reorder serves POST /orders/{id}/reorder — rebuilds the cart from the
// order's items at current prices, honouring availability and the
// single-restaurant cart rule. Modifiers are not re-applied automatically;
// the customer can customise items again.
func (h *Handlers) Reorder(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	orderID := r.PathValue("id")

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	// the caller's order + its outlet
	var outletID string
	err = tx.QueryRow(r.Context(), `
		SELECT outlet_id FROM orders WHERE id = $1 AND customer_id = $2`,
		orderID, p.ID).Scan(&outletID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Order not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// existing cart handling (cross-outlet requires explicit replacement —
	// we replace when the reorder source is a different outlet, mirroring the
	// cart conflict flow but scoped to a deliberate user action)
	var cartID, cartOutlet string
	err = tx.QueryRow(r.Context(),
		`SELECT id, outlet_id FROM carts WHERE user_id = $1 AND status = 'active'`, p.ID).
		Scan(&cartID, &cartOutlet)
	replaced := false
	cartExists := err == nil
	if err != nil && err.Error() != "no rows in result set" {
		httpx.WriteError(w, r, err)
		return
	}
	if cartExists && cartOutlet != outletID {
		if _, err = tx.Exec(r.Context(), `DELETE FROM cart_items WHERE cart_id = $1`, cartID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(),
			`UPDATE carts SET outlet_id = $2, updated_at = now() WHERE id = $1`, cartID, outletID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		replaced = true
	}
	if !cartExists {
		err = tx.QueryRow(r.Context(),
			`INSERT INTO carts (user_id, outlet_id) VALUES ($1,$2) RETURNING id`, p.ID, outletID).Scan(&cartID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}

	// re-add items that are still on the menu and available, at live prices
	rows, err := tx.Query(r.Context(), `
		SELECT oi.menu_item_id, oi.quantity
		FROM order_items oi
		JOIN menu_items mi ON mi.id = oi.menu_item_id
		WHERE oi.order_id = $1`,
		orderID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	type line struct {
		itemID string
		qty    int
	}
	var lines []line
	for rows.Next() {
		var l line
		if rows.Scan(&l.itemID, &l.qty) == nil {
			lines = append(lines, l)
		}
	}
	rows.Close()
	if rows.Err() != nil {
		httpx.WriteError(w, r, rows.Err())
		return
	}

	added := 0
	for _, l := range lines {
		var name string
		var price int64
		var veg bool
		err := tx.QueryRow(r.Context(), `
			SELECT name, price_paise, is_veg FROM menu_items WHERE id = $1 AND is_available AND deleted_at IS NULL`,
			l.itemID).Scan(&name, &price, &veg)
		if err != nil {
			continue // disappeared between check and insert
		}
		if _, err = tx.Exec(r.Context(), `
			INSERT INTO cart_items (cart_id, menu_item_id, quantity, name_snapshot, price_snapshot_paise, is_veg_snapshot)
			VALUES ($1,$2,$3,$4,$5,$6)`, cartID, l.itemID, l.qty, name, price, veg); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		added++
	}
	skipped := len(lines) - added

	if _, err = tx.Exec(r.Context(),
		`UPDATE carts SET updated_at = now() WHERE id = $1`, cartID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteData(w, r, http.StatusCreated, reorderResult{
		CartID:     cartID,
		OutletID:   outletID,
		Added:      added,
		Skipped:    skipped,
		SkippedWhy: ternaryStr(skipped > 0, "Some items are no longer available; they were skipped."),
		Replaced:   replaced,
	})
}

func ternaryStr(cond bool, a string) string {
	if cond {
		return a
	}
	return ""
}

var _ = apperr.CodeUnprocessable
