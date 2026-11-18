package seed

import "fmt"

// seedCoupons creates synthetic promo codes covering valid, expired,
// minimum-not-met and new-user-only states (all clearly demo-labelled).
func (s *seeder) seedCoupons() error {
	coupons := []struct {
		code, title, desc string
		kind              string // percent | fixed
		percentBP         int    // basis points
		fixedPaise        int64
		capPaise          int64
		minOrder          int64
		startsOffsetDays  int // negative = past
		endsOffsetDays    int // <=0 = no end / already ended when negative
		perUser           int
		funding           string
		outletIdx         int // -1 = all outlets
		newUsersOnly      bool
	}{
		{"ORBIT50", "₹50 off on orders above ₹299", "Demo coupon — platform funded", "fixed", 0, 5000, 0, 29900, -7, 60, 2, "platform", -1, false},
		{"FLY20", "20% off up to ₹120", "Demo coupon — platform funded", "percent", 2000, 0, 12000, 24900, -7, 60, 3, "platform", -1, false},
		{"NEWBITE", "Flat ₹100 for first BiteOrbit order", "Demo coupon — new users only", "fixed", 0, 10000, 0, 19900, -7, 90, 1, "platform", -1, true},
		{"TANDOOR15", "15% off at Tandoor Tales", "Demo coupon — restaurant funded", "percent", 1500, 0, 10000, 39900, -7, 45, 1, "restaurant", 1, false},
		{"SALENI", "Expired demo coupon (kept for testing)", "Demo coupon — expired", "percent", 3000, 0, 20000, 0, -30, -5, 5, "platform", -1, false},
	}
	for i, c := range coupons {
		var outletID any
		if c.outletIdx >= 0 && c.outletIdx < len(s.outletIDs) {
			outletID = s.outletIDs[c.outletIdx]
		}
		var endsSQL string
		if c.endsOffsetDays > 0 {
			endsSQL = fmt.Sprintf("now() + interval '%d days'", c.endsOffsetDays)
		} else if c.endsOffsetDays < 0 {
			endsSQL = fmt.Sprintf("now() - interval '%d days'", -c.endsOffsetDays)
		} else {
			endsSQL = "NULL"
		}
		query := `
			INSERT INTO coupons
			(code, title, description, discount_type, percent_bp, fixed_paise, max_cap_paise, min_order_paise,
			 starts_at, ends_at, is_active, usage_limit_per_user, funding_party, eligible_outlet_id, new_users_only, created_by)
			VALUES ($1,$2,$3,$4,NULLIF($5,0),NULLIF($6,0),$7,$8,
			 now() - interval '7 days',
			 ` + endsSQL + `,
			 $9,$10,$11,$12,$13,$14)`
		if _, err := s.tx.Exec(s.ctx, query,
			c.code, c.title, c.desc, c.kind, c.percentBP, c.fixedPaise, c.capPaise, c.minOrder,
			c.endsOffsetDays >= 0, c.perUser, c.funding, outletID, c.newUsersOnly, s.adminID); err != nil {
			return err
		}
		_ = i
	}
	return nil
}
