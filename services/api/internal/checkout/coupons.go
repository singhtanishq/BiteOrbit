package checkout

import (
	"errors"
	"net/http"

	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/pricing"
)

type validateCouponBody struct {
	Code string `json:"code"`
}

type validateCouponResponse struct {
	Valid         bool   `json:"valid"`
	Code          string `json:"code,omitempty"`
	Title         string `json:"title,omitempty"`
	DiscountPaise int64  `json:"discount_paise,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Message       string `json:"message,omitempty"`
}

// ValidateCoupon checks a coupon against the caller's CURRENT cart subtotal
// (server-computed) and returns a user-friendly result either way.
func (h *Handlers) ValidateCoupon(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body validateCouponBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	st, err := loadCartState(r.Context(), h.pool, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	coupon, cErr := pricing.ValidateCoupon(r.Context(), h.pool, body.Code, st.outletID, st.subtotal, p.ID)
	if cErr != nil {
		var ce *pricing.CouponError
		if errors.As(cErr, &ce) {
			httpx.WriteData(w, r, http.StatusOK, validateCouponResponse{
				Valid:   false,
				Reason:  ce.Code,
				Message: ce.Message,
			})
			return
		}
		httpx.WriteError(w, r, cErr)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, validateCouponResponse{
		Valid:         true,
		Code:          coupon.Code,
		Title:         coupon.Title,
		DiscountPaise: coupon.DiscountPaise,
	})
}
