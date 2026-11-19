package portal

import (
	"net/http"
	"time"

	"biteorbit/services/api/internal/httpx"
)

// AnalyticsSummary serves GET /restaurant-portal/analytics?outlet_id=&days=.
// Metric definitions (documented, test-covered):
//
//	orders          — count of orders excluding pending_payment
//	gov_paise       — GROSS ORDER VALUE: grand totals of delivered orders only.
//	                  NOT restaurant payout or net revenue.
//	net_sales_paise — item subtotals minus coupon discounts on delivered orders
//	                  (before commissions/settlement adjustments)
//	aov_paise       — net sales / delivered orders (0 when none)
//	cancelled_rate  — cancelled+rejected / all non-pending orders
//	avg_prep_minutes — mean of (ready_at − preparing_at) over delivered orders
type AnalyticsSummary struct {
	Orders         int64     `json:"orders"`
	Delivered      int64     `json:"delivered"`
	Cancelled      int64     `json:"cancelled"`
	Rejected       int64     `json:"rejected"`
	GovPaise       int64     `json:"gov_paise"`
	NetSalesPaise  int64     `json:"net_sales_paise"`
	AOVPaise       int64     `json:"aov_paise"`
	CancelRateBP   int64     `json:"cancel_rate_bp"`
	AvgPrepMinutes int64     `json:"avg_prep_minutes"`
	TopItems       []topItem `json:"top_items"`
	DataFreshness  time.Time `json:"data_as_of"`
	DemoNote       string    `json:"demo_note"`
}

type topItem struct {
	Name    string `json:"name"`
	Qty     int64  `json:"qty"`
	Revenue int64  `json:"revenue_paise"`
}

// Analytics serves the summary for one outlet over the last N days.
func (h *Handlers) Analytics(w http.ResponseWriter, r *http.Request) {
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
	days := atoiDefault(r.URL.Query().Get("days"), 1)
	if days < 1 || days > 90 {
		days = 1
	}

	var s AnalyticsSummary
	var deliveredForAOV int64
	err := h.pool.QueryRow(r.Context(), `
		SELECT
			COUNT(*) FILTER (WHERE status NOT IN ('pending_payment')),
			COUNT(*) FILTER (WHERE status = 'delivered'),
			COUNT(*) FILTER (WHERE status IN ('cancelled','cancel_requested')),
			COUNT(*) FILTER (WHERE status = 'rejected'),
			COALESCE(SUM(grand_total_paise) FILTER (WHERE status = 'delivered'), 0),
			COALESCE(SUM(items_subtotal_paise - coupon_discount_paise) FILTER (WHERE status = 'delivered'), 0),
			COALESCE((SELECT ROUND(AVG(EXTRACT(EPOCH FROM (ready_at - preparing_at))/60))
				FROM orders WHERE outlet_id = $1 AND status='delivered'
				AND preparing_at IS NOT NULL AND ready_at IS NOT NULL
				AND created_at > now() - ($2 || ' days')::interval), 0)
		FROM orders
		WHERE outlet_id = $1 AND created_at > now() - ($2 || ' days')::interval`,
		outletID, itoa(days)).Scan(
		&s.Orders, &s.Delivered, &s.Cancelled, &s.Rejected,
		&s.GovPaise, &s.NetSalesPaise, &s.AvgPrepMinutes)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	deliveredForAOV = s.Delivered
	if deliveredForAOV > 0 {
		s.AOVPaise = s.NetSalesPaise / deliveredForAOV
	}
	if s.Orders > 0 {
		s.CancelRateBP = (s.Cancelled + s.Rejected) * 10000 / s.Orders
	}
	s.DataFreshness = time.Now().UTC()
	s.DemoNote = "Synthetic demo data. GOV is gross order value — not restaurant payout."

	// top items by quantity on delivered orders
	rows, err := h.pool.Query(r.Context(), `
		SELECT oi.name_snapshot, SUM(oi.quantity), SUM(oi.item_total_paise)
		FROM order_items oi JOIN orders o ON o.id = oi.order_id
		WHERE o.outlet_id = $1 AND o.status = 'delivered'
			AND o.created_at > now() - ($2 || ' days')::interval
		GROUP BY oi.name_snapshot ORDER BY SUM(oi.quantity) DESC LIMIT 5`,
		outletID, itoa(days))
	if err == nil {
		s.TopItems = []topItem{}
		for rows.Next() {
			var t topItem
			if rows.Scan(&t.Name, &t.Qty, &t.Revenue) == nil {
				s.TopItems = append(s.TopItems, t)
			}
		}
		rows.Close()
	}

	httpx.WriteData(w, r, http.StatusOK, s)
}
