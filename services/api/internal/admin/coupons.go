package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"biteorbit/services/api/internal/httpx"
)

// ListCoupons serves GET /admin/coupons.
func (h *Handlers) ListCoupons(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, code, title, discount_type, percent_bp, fixed_paise, max_cap_paise,
			min_order_paise, starts_at, ends_at, is_active, usage_limit_per_user,
			total_redemption_limit, redeemed_count, funding_party, new_users_only
		FROM coupons ORDER BY created_at DESC`)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type couponRow struct {
		ID         string     `json:"id"`
		Code       string     `json:"code"`
		Title      string     `json:"title"`
		Type       string     `json:"discount_type"`
		PercentBP  *int64     `json:"percent_bp"`
		Fixed      *int64     `json:"fixed_paise"`
		Cap        *int64     `json:"max_cap_paise"`
		MinOrder   int64      `json:"min_order_paise"`
		StartsAt   time.Time  `json:"starts_at"`
		EndsAt     *time.Time `json:"ends_at"`
		Active     bool       `json:"is_active"`
		PerUser    int        `json:"usage_limit_per_user"`
		TotalLimit *int64     `json:"total_redemption_limit"`
		Redeemed   int        `json:"redeemed_count"`
		Funding    string     `json:"funding_party"`
		NewUsers   bool       `json:"new_users_only"`
	}
	out := []couponRow{}
	for rows.Next() {
		var c couponRow
		if rows.Scan(&c.ID, &c.Code, &c.Title, &c.Type, &c.PercentBP, &c.Fixed, &c.Cap,
			&c.MinOrder, &c.StartsAt, &c.EndsAt, &c.Active, &c.PerUser, &c.TotalLimit,
			&c.Redeemed, &c.Funding, &c.NewUsers) == nil {
			out = append(out, c)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

type createCouponBody struct {
	Code                 string `json:"code"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	DiscountType         string `json:"discount_type"` // percent | fixed
	PercentBP            *int64 `json:"percent_bp"`
	FixedPaise           *int64 `json:"fixed_paise"`
	MaxCapPaise          *int64 `json:"max_cap_paise"`
	MinOrderPaise        int64  `json:"min_order_paise"`
	ValidDays            int    `json:"valid_days"`
	UsageLimitPerUser    int    `json:"usage_limit_per_user"`
	TotalRedemptionLimit *int64 `json:"total_redemption_limit"`
	FundingParty         string `json:"funding_party"`
	NewUsersOnly         bool   `json:"new_users_only"`
}

// CreateCoupon serves POST /admin/coupons — audited.
func (h *Handlers) CreateCoupon(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	if isSupportOnly(p.Roles) {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden", "Support agents cannot create coupons.", nil)
		return
	}
	var body createCouponBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	switch body.DiscountType {
	case "percent":
		if body.PercentBP == nil || *body.PercentBP < 1 || *body.PercentBP > 10000 {
			httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
				"percent_bp must be 1–10000 for percent coupons.", nil)
			return
		}
	case "fixed":
		if body.FixedPaise == nil || *body.FixedPaise < 100 {
			httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
				"fixed_paise must be at least ₹1 for fixed coupons.", nil)
			return
		}
	default:
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"discount_type must be percent or fixed.", nil)
		return
	}
	if body.Code == "" || len(body.Code) > 30 || body.Title == "" {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"A code (≤30 chars) and title are required.", nil)
		return
	}
	if body.FundingParty == "" {
		body.FundingParty = "platform"
	}
	if body.ValidDays <= 0 || body.ValidDays > 365 {
		body.ValidDays = 30
	}
	if body.UsageLimitPerUser < 1 {
		body.UsageLimitPerUser = 1
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO coupons
		(code, title, description, discount_type, percent_bp, fixed_paise, max_cap_paise, min_order_paise,
		 starts_at, ends_at, is_active, usage_limit_per_user, total_redemption_limit, funding_party, new_users_only, created_by)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8, now(), now() + ($9 || ' days')::interval, true, $10,$11,$12,$13,$14)
		RETURNING id`,
		body.Code, body.Title, body.Description, body.DiscountType, body.PercentBP, body.FixedPaise,
		body.MaxCapPaise, body.MinOrderPaise, itoa(body.ValidDays), body.UsageLimitPerUser,
		body.TotalRedemptionLimit, body.FundingParty, body.NewUsersOnly, p.ID).Scan(&id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = audit(r.Context(), tx, p.ID, "coupon.created", "coupon", id,
		nil, map[string]any{"code": body.Code, "type": body.DiscountType}, "Coupon created via admin console"); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{"id": id, "code": body.Code})
}

type couponFlagBody struct {
	IsActive bool   `json:"is_active"`
	Reason   string `json:"reason"`
}

// UpdateCoupon serves PATCH /admin/coupons/{id} — activate/deactivate.
func (h *Handlers) UpdateCoupon(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	if isSupportOnly(p.Roles) {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden", "Support agents cannot modify coupons.", nil)
		return
	}
	id := r.PathValue("id")
	var body couponFlagBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(body.Reason) < 4 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "A reason is required.", nil)
		return
	}
	var before bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT is_active FROM coupons WHERE id = $1`, id).Scan(&before)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Coupon not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = h.pool.Exec(r.Context(),
		`UPDATE coupons SET is_active=$2 WHERE id=$1`, id, body.IsActive); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = audit(r.Context(), h.pool, p.ID, "coupon.updated", "coupon", id,
		map[string]any{"is_active": before}, map[string]any{"is_active": body.IsActive}, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"is_active": body.IsActive})
}
