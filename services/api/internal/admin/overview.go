// Package admin implements the platform administration console API. Every
// mutation is audited (actor, target, before/after, reason) and every route
// is role-guarded; support agents get reduced permissions.
package admin

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/httpx"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

// hasRole reports whether the caller holds one of the roles (RBAC enforcement
// happens in middleware; this refines support_agent limitations).
func isSupportOnly(roles []string) bool {
	hasSupport := false
	hasOther := false
	for _, r := range roles {
		if r == "support_agent" {
			hasSupport = true
		}
		if r == "admin" || r == "ops_manager" {
			hasOther = true
		}
	}
	return hasSupport && !hasOther
}

// Overview serves GET /admin/overview?days=&city= — platform-wide metrics
// with documented definitions. Data derives from persisted orders only.
func (h *Handlers) Overview(w http.ResponseWriter, r *http.Request) {
	days := atoiDefault(r.URL.Query().Get("days"), 7)
	if days < 1 || days > 90 {
		days = 7
	}
	city := r.URL.Query().Get("city")

	var m struct {
		OrdersTotal       int64            `json:"orders_total"`
		OrdersByStatus    map[string]int64 `json:"orders_by_status"`
		GovPaise          int64            `json:"gov_paise"`
		PlatformFeesPaise int64            `json:"platform_fees_paise"`
		DeliveryFeesPaise int64            `json:"delivery_fees_paise"`
		RefundsPaise      int64            `json:"refunds_paise"`
		CouponCostPaise   int64            `json:"coupon_cost_paise"`
		ActiveOutlets     int64            `json:"active_outlets"`
		PendingApps       int64            `json:"pending_applications"`
		RidersOnline      int64            `json:"riders_online"`
		RidersTotal       int64            `json:"riders_total"`
		UsersTotal        int64            `json:"users_total"`
		TicketsOpen       int64            `json:"tickets_open"`
		DataAsOf          string           `json:"data_as_of"`
		DemoNote          string           `json:"demo_note"`
	}
	cityFilter := ""
	args := []any{strconv.Itoa(days)}
	if city != "" {
		args = append(args, city)
		cityFilter = " AND ot.city = $" + strconv.Itoa(len(args))
	}

	statusRows, err := h.pool.Query(r.Context(), `
		SELECT o.status, COUNT(*)
		FROM orders o JOIN outlets ot ON ot.id = o.outlet_id
		WHERE o.created_at > now() - ($1 || ' days')::interval`+cityFilter+`
		GROUP BY o.status`, args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	m.OrdersByStatus = map[string]int64{}
	for statusRows.Next() {
		var s string
		var n int64
		if statusRows.Scan(&s, &n) == nil {
			m.OrdersByStatus[s] = n
			m.OrdersTotal += n
		}
	}
	statusRows.Close()

	err = h.pool.QueryRow(r.Context(), `
		SELECT
			COALESCE(SUM(o.grand_total_paise) FILTER (WHERE o.status = 'delivered'), 0),
			COALESCE(SUM(o.platform_fee_paise) FILTER (WHERE o.status = 'delivered'), 0),
			COALESCE(SUM(o.delivery_fee_paise) FILTER (WHERE o.status = 'delivered'), 0),
			COALESCE(SUM(o.coupon_discount_paise) FILTER (WHERE o.status = 'delivered'), 0)
		FROM orders o JOIN outlets ot ON ot.id = o.outlet_id
		WHERE o.created_at > now() - ($1 || ' days')::interval`+cityFilter, args...).
		Scan(&m.GovPaise, &m.PlatformFeesPaise, &m.DeliveryFeesPaise, &m.CouponCostPaise)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	_ = h.pool.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(amount_paise), 0) FROM refunds
		WHERE created_at > now() - ($1 || ' days')::interval`, args...).Scan(&m.RefundsPaise)

	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM outlets WHERE status = 'active' AND deleted_at IS NULL`).Scan(&m.ActiveOutlets)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM restaurant_applications WHERE status IN ('submitted','under_review')`).Scan(&m.PendingApps)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM delivery_partners WHERE availability = 'online' AND status='approved'`).Scan(&m.RidersOnline)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM delivery_partners`).Scan(&m.RidersTotal)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`).Scan(&m.UsersTotal)
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM support_tickets WHERE status IN ('open','assigned','waiting_for_customer')`).Scan(&m.TicketsOpen)

	m.DataAsOf = clockNow()
	m.DemoNote = "Synthetic demo metrics. GOV = gross order value of delivered orders; platform fees are NOT net revenue."
	httpx.WriteData(w, r, http.StatusOK, m)
}

func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
