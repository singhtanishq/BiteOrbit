package orders

// CSV exports (P1): restaurant sales report and admin operations report.
// Exports stream as text/csv with a filename; they contain only operational
// fields — no customer personal data beyond what the role may already see.

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/httpx"
)

// RestaurantSalesExport serves GET /restaurant-portal/analytics/export.csv
// — per-day sales summary for the outlet (GOV/net sales/AOV defined in the
// analytics endpoint). Staff-scoped via the portal handler's rules.
func RestaurantSalesExport(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := httpx.UserFrom(r.Context())
		outletID := r.URL.Query().Get("outlet_id")
		if outletID == "" {
			httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "outlet_id is required.", nil)
			return
		}
		// authorisation mirrors the portal handler
		var n int
		if err := pool.QueryRow(r.Context(), `
			SELECT COUNT(*) FROM outlets o
			JOIN restaurant_organisations org ON org.id = o.organisation_id
			LEFT JOIN outlet_staff os ON os.outlet_id = o.id AND os.user_id = $1
			WHERE o.id = $2 AND (org.owner_user_id = $1 OR os.user_id IS NOT NULL)`,
			p.ID, outletID).Scan(&n); err != nil || n == 0 {
			httpx.Fail(w, r, http.StatusNotFound, "not_found", "Outlet not found or not accessible.", nil)
			return
		}

		rows, err := pool.Query(r.Context(), `
			SELECT date_trunc('day', COALESCE(o.placed_at, o.created_at))::date::text,
				COUNT(*),
				COALESCE(SUM(o.grand_total_paise) FILTER (WHERE o.status = 'delivered'), 0),
				COALESCE(SUM(o.items_subtotal_paise - o.coupon_discount_paise) FILTER (WHERE o.status = 'delivered'), 0),
				COUNT(*) FILTER (WHERE o.status IN ('cancelled','cancel_requested','rejected'))
			FROM orders o
			WHERE o.outlet_id = $1 AND o.created_at > now() - interval '30 days'
			GROUP BY 1 ORDER BY 1 DESC`, outletID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		defer rows.Close()

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=biteorbit-sales-%s.csv", time.Now().UTC().Format("20060102")))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "day,orders,gross_order_value_paise,net_sales_paise,cancelled_or_rejected")
		for rows.Next() {
			var day string
			var orders, gov, net, cancelled int64
			if err := rows.Scan(&day, &orders, &gov, &net, &cancelled); err != nil {
				return
			}
			fmt.Fprintf(w, "%s,%d,%d,%d,%d\n", day, orders, gov, net, cancelled)
		}
	}
}

// AdminOrdersExport serves GET /admin/orders/export.csv — ops/manager only.
func AdminOrdersExport(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := pool.Query(r.Context(), `
			SELECT o.reference, o.status, ot.name, o.grand_total_paise,
				o.payment_method_hint, COALESCE(to_char(o.placed_at, 'YYYY-MM-DD HH24:MI'), '')
			FROM (
				SELECT o.*, (SELECT method FROM payments pm WHERE pm.order_id = o.id ORDER BY created_at DESC LIMIT 1) AS payment_method_hint
				FROM orders o
			) o
			JOIN outlets ot ON ot.id = o.outlet_id
			WHERE o.created_at > now() - interval '30 days'
			ORDER BY o.created_at DESC LIMIT 5000`)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		defer rows.Close()

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition",
			fmt.Sprintf("attachment; filename=biteorbit-orders-%s.csv", time.Now().UTC().Format("20060102")))
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "reference,status,outlet,grand_total_paise,payment_method,placed_at")
		for rows.Next() {
			var reference, status, outlet, method, placed string
			var total int64
			if err := rows.Scan(&reference, &status, &outlet, &total, &method, &placed); err != nil {
				return
			}
			fmt.Fprintf(w, "%q,%s,%q,%d,%s,%s\n", reference, status, outlet, total, method, placed)
		}
	}
}
