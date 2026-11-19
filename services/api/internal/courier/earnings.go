package courier

import (
	"encoding/json"
	"net/http"
	"time"

	"biteorbit/services/api/internal/httpx"
)

func jsonMarshal(m map[string]any) ([]byte, error) { return json.Marshal(m) }

// Earnings serves GET /courier/earnings — persisted earnings only, split by
// day. Riders cannot edit amounts; settlements are admin actions.
func (h *Handlers) Earnings(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	riderID, err := h.riderOf(r.Context(), h.pool, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	type dayRow struct {
		Day        string `json:"day"`
		Deliveries int64  `json:"deliveries"`
		Base       int64  `json:"base_paise"`
		Incentive  int64  `json:"incentive_paise"`
		Tips       int64  `json:"tip_paise"`
		Total      int64  `json:"total_paise"`
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT date_trunc('day', re.created_at)::date::text, COUNT(*),
			SUM(re.base_pay_paise), SUM(re.incentive_paise), SUM(re.tip_paise), SUM(re.total_paise)
		FROM rider_earnings re
		WHERE re.rider_id = $1
		GROUP BY 1 ORDER BY 1 DESC LIMIT 30`, riderID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	days := []dayRow{}
	var totalAll, totalTips int64
	var deliveriesAll int64
	for rows.Next() {
		var d dayRow
		if rows.Scan(&d.Day, &d.Deliveries, &d.Base, &d.Incentive, &d.Tips, &d.Total) == nil {
			days = append(days, d)
			totalAll += d.Total
			totalTips += d.Tips
			deliveriesAll += d.Deliveries
		}
	}

	// today's deliveries from assignments
	var todayDeliveries int64
	_ = h.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM delivery_assignments
		WHERE rider_id = $1 AND status = 'delivered'
		AND delivered_at = (SELECT date_trunc('day', now()))`, riderID).Scan(&todayDeliveries)
	_ = time.Now

	// current availability
	var availability string
	_ = h.pool.QueryRow(r.Context(),
		`SELECT availability FROM delivery_partners WHERE id = $1`, riderID).Scan(&availability)

	httpx.WriteData(w, r, http.StatusOK, map[string]any{
		"days": days,
		"totals": map[string]any{
			"deliveries":  deliveriesAll,
			"total_paise": totalAll,
			"tip_paise":   totalTips,
			"status":      "payable (settled by platform — not rider-editable)",
		},
		"today_deliveries": todayDeliveries,
		"availability":     availability,
		"demo_note":        "Earnings derive from the persisted demo ledger. Payout/settlement is a platform action riders cannot perform.",
	})
}
