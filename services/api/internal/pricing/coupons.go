package pricing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"biteorbit/services/api/internal/db"
)

// Coupon is the validated, discount-ready coupon view.
type Coupon struct {
	ID            string `json:"id"`
	Code          string `json:"code"`
	Title         string `json:"title"`
	DiscountPaise int64  `json:"discount_paise"`
	FundingParty  string `json:"funding_party"`
}

// CouponError codes surface specific, user-friendly reasons.
type CouponError struct {
	Code    string // unknown | inactive | expired | not_started | outlet_mismatch | min_order | user_limit | global_limit | new_users_only
	Message string
}

func (e *CouponError) Error() string { return e.Message }

// ValidateCoupon checks a coupon against the CURRENT server-computed state.
// It never trusts client-submitted subtotals or outlet ids.
func ValidateCoupon(ctx context.Context, q db.Querier, code, outletID string, itemsSubtotalPaise int64, userID string) (*Coupon, error) {
	var c struct {
		id                   string
		code                 string
		title                string
		discountType         string
		percentBP            *int64
		fixedPaise           *int64
		maxCapPaise          *int64
		minOrder             int64
		startsAt             time.Time
		endsAt               *time.Time
		isActive             bool
		usageLimitPerUser    int
		totalRedemptionLimit *int64
		redeemedCount        int
		fundingParty         string
		eligibleOutletID     *string
		newUsersOnly         bool
	}
	err := q.QueryRow(ctx, `
		SELECT id, code, title, discount_type, percent_bp, fixed_paise, max_cap_paise, min_order_paise,
			starts_at, ends_at, is_active, usage_limit_per_user, total_redemption_limit, redeemed_count,
			funding_party, eligible_outlet_id, new_users_only
		FROM coupons WHERE code = $1`, code).Scan(
		&c.id, &c.code, &c.title, &c.discountType, &c.percentBP, &c.fixedPaise, &c.maxCapPaise,
		&c.minOrder, &c.startsAt, &c.endsAt, &c.isActive, &c.usageLimitPerUser,
		&c.totalRedemptionLimit, &c.redeemedCount, &c.fundingParty, &c.eligibleOutletID, &c.newUsersOnly)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &CouponError{Code: "unknown", Message: "That coupon code doesn't exist."}
	}
	if err != nil {
		return nil, &CouponError{Code: "unknown", Message: "Coupon lookup failed."}
	}
	now := time.Now()
	switch {
	case !c.isActive:
		return nil, &CouponError{Code: "inactive", Message: "This coupon is no longer active."}
	case c.startsAt.After(now):
		return nil, &CouponError{Code: "not_started", Message: "This coupon isn't active yet."}
	case c.endsAt != nil && c.endsAt.Before(now):
		return nil, &CouponError{Code: "expired", Message: "This coupon has expired."}
	case c.eligibleOutletID != nil && *c.eligibleOutletID != outletID:
		return nil, &CouponError{Code: "outlet_mismatch", Message: "This coupon applies to a different restaurant."}
	case itemsSubtotalPaise < c.minOrder:
		return nil, &CouponError{Code: "min_order", Message: fmt.Sprintf(
			"Add items worth ₹%.2f more to use this coupon.", float64(c.minOrder-itemsSubtotalPaise)/100)}
	case c.totalRedemptionLimit != nil && c.redeemedCount >= int(*c.totalRedemptionLimit):
		return nil, &CouponError{Code: "global_limit", Message: "This coupon has been fully redeemed."}
	}

	// per-user redemption count from actual redemptions
	var userCount int
	if err := q.QueryRow(ctx, `
		SELECT COUNT(*) FROM coupon_redemptions WHERE coupon_id = $1 AND user_id = $2`,
		c.id, userID).Scan(&userCount); err != nil {
		return nil, &CouponError{Code: "unknown", Message: "Coupon lookup failed."}
	}
	if userCount >= c.usageLimitPerUser {
		return nil, &CouponError{Code: "user_limit", Message: "You've already used this coupon the maximum number of times."}
	}

	if c.newUsersOnly {
		var priorOrders int
		if err := q.QueryRow(ctx, `
			SELECT COUNT(*) FROM orders
			WHERE customer_id = $1 AND status NOT IN ('pending_payment')
			  AND status NOT IN ('rejected')
			  AND status NOT IN ('cancelled')`, userID).Scan(&priorOrders); err != nil {
			return nil, &CouponError{Code: "unknown", Message: "Coupon lookup failed."}
		}
		if priorOrders > 0 {
			return nil, &CouponError{Code: "new_users_only", Message: "This coupon is only for first BiteOrbit orders."}
		}
	}

	var discount int64
	if c.discountType == "percent" && c.percentBP != nil {
		discount = itemsSubtotalPaise * *c.percentBP / 10000
		if c.maxCapPaise != nil && discount > *c.maxCapPaise {
			discount = *c.maxCapPaise
		}
	} else if c.fixedPaise != nil {
		discount = *c.fixedPaise
	}
	if discount > itemsSubtotalPaise {
		discount = itemsSubtotalPaise
	}
	return &Coupon{ID: c.id, Code: c.code, Title: c.title, DiscountPaise: discount, FundingParty: c.fundingParty}, nil
}

// RedeemCoupon atomically records a redemption and increments the counter.
// Run inside the placement transaction; the unique order_id index plus the
// conditional UPDATE make duplicate/concurrent redemption safe.
func RedeemCoupon(ctx context.Context, q db.Querier, couponID, userID, orderID string, discountPaise int64) error {
	var redeemed int
	err := q.QueryRow(ctx, `
		UPDATE coupons SET redeemed_count = redeemed_count + 1
		WHERE id = $1 AND (total_redemption_limit IS NULL OR redeemed_count < total_redemption_limit)
		RETURNING redeemed_count`, couponID).Scan(&redeemed)
	if errors.Is(err, pgx.ErrNoRows) {
		return &CouponError{Code: "global_limit", Message: "This coupon just ran out."}
	}
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO coupon_redemptions (coupon_id, user_id, order_id, discount_paise)
		VALUES ($1,$2,$3,$4)`, couponID, userID, orderID, discountPaise)
	return err
}
