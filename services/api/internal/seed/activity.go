package seed

import (
	"fmt"
	"time"
)

// seedActivity synthesises historical orders across every lifecycle state so
// dashboards, tracking and admin views have realistic data from first boot.
func (s *seeder) seedActivity(count int) error {
	for i := 0; i < count; i++ {
		if err := s.seedOneOrder(i); err != nil {
			return err
		}
	}

	// favourites & search history
	for ci, uid := range s.customerIDs {
		for k := 0; k < 2; k++ {
			outlet := s.outletIDs[(ci+k*3)%len(s.outletIDs)]
			if _, err := s.tx.Exec(s.ctx,
				`INSERT INTO favourites (user_id, outlet_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
				uid, outlet); err != nil {
				return err
			}
		}
		queries := []string{"biryani", "dosa", "momos", "pizza", "thal", "roll", "coffee"}
		for k := 0; k < 3; k++ {
			if _, err := s.tx.Exec(s.ctx,
				`INSERT INTO search_history (user_id, query) VALUES ($1,$2)`,
				uid, queries[(ci+k)%len(queries)]); err != nil {
				return err
			}
		}
	}
	return nil
}

// status plan (deterministic distribution):
// delivered 62%, cancelled 10%, rejected 8%, delivery_failed 3%,
// on_the_way 5%, preparing 6%, restaurant_confirmed 4%, rider_assigned 2%.
func statusFor(i int) string {
	switch i % 100 {
	case 0, 1, 2, 3, 4, 5, 6, 7:
		return "cancelled"
	case 10, 11, 12, 13, 14, 15, 16, 17:
		return "rejected"
	case 20, 21:
		return "delivery_failed"
	case 30, 31, 32, 33, 34:
		return "on_the_way"
	case 40, 41, 42, 43, 44, 45:
		return "preparing"
	case 50, 51, 52, 53:
		return "restaurant_confirmed"
	case 60, 61:
		return "rider_assigned"
	default:
		return "delivered"
	}
}

func (s *seeder) seedOneOrder(seq int) error {
	customerID := s.customerIDs[seq%len(s.customerIDs)]
	outletID := s.outletIDs[s.rng.Intn(len(s.outletIDs))]
	status := statusFor(seq)

	// recent orders are likelier to be in live states
	if seq < 20 && (status == "delivered") {
		status = pick(s.rng, "preparing", "on_the_way", "restaurant_confirmed", "rider_assigned")
	}

	items := s.pickOrderItems(outletID, 1+s.rng.Intn(3))
	if len(items) == 0 {
		return nil
	}

	subtotal := 0
	for _, it := range items {
		subtotal += it.total
	}
	// outlet packaging fee (mirrors seeded outlet config; read it back)
	var packaging int64
	if err := s.tx.QueryRow(s.ctx,
		`SELECT packaging_fee_paise FROM outlets WHERE id = $1`, outletID).Scan(&packaging); err != nil {
		return err
	}

	// pricing approximation consistent with pricing rule v1:
	// delivery 29 + 9/km capped 99, free ≥599; platform 4% cap ₹25;
	// small-order ₹30 below ₹199 subtotal.
	deliveryFee := int64(2900)
	if subtotal >= 59900 {
		deliveryFee = 0
	}
	platformFee := int64(subtotal) * 400 / 10000
	if platformFee > 2500 {
		platformFee = 2500
	}
	smallOrderFee := int64(0)
	if subtotal < 19900 {
		smallOrderFee = 3000
	}
	tax := int64(0)
	for _, it := range items {
		if it.tax == "gst_5" {
			tax += int64(it.total) * 500 / 10000
		} else if it.tax == "gst_12" {
			tax += int64(it.total) * 1200 / 10000
		}
	}

	// coupon on ~25% of eligible orders
	var couponDiscount int64
	var couponID any
	if seq%4 == 1 && subtotal >= 29900 {
		var code string
		if err := s.tx.QueryRow(s.ctx,
			`SELECT id, code FROM coupons WHERE code='ORBIT50' AND ends_at > now() LIMIT 1`).Scan(&couponID, &code); err == nil {
			couponDiscount = 5000
		}
	}

	grand := int64(subtotal) - couponDiscount + packaging + deliveryFee + platformFee + smallOrderFee + tax

	// timestamps: spread across past 30 days, recent for live states
	ageDays := 30
	if isLive(status) {
		ageDays = s.rng.Intn(2)
	} else {
		ageDays = s.rng.Intn(30)
	}
	placedAt := time.Now().Add(-time.Duration(ageDays)*24*time.Hour - time.Duration(s.rng.Intn(12))*time.Hour)

	instructions := ""
	if s.rng.Intn(6) == 0 {
		instructions = "Extra spicy; less oil please."
	}
	orderID := s.mustInsert(`
		INSERT INTO orders
		(reference, customer_id, outlet_id, organisation_id, status, mode, placed_at,
		 items_subtotal_paise, coupon_discount_paise, packaging_fee_paise, delivery_fee_paise,
		 platform_fee_paise, small_order_fee_paise, tax_paise, grand_total_paise,
		 pricing_snapshot, instructions, eta_min_minutes, eta_max_minutes, created_at, updated_at)
		VALUES ($1,$2,$3,(SELECT organisation_id FROM outlets WHERE id=$3),$4,'delivery',$5,
		 $6,$7,$8,$9,$10,$11,$12,$13,
		 '{"rule_version":1,"rule":"demo-pricing-v1","seeded":true}'::jsonb, $14, $15, $16, $5, now())
		RETURNING id`,
		fmt.Sprintf("BO-%05d", 10001+seq), customerID, outletID, status, placedAt,
		subtotal, couponDiscount, packaging, deliveryFee, platformFee, smallOrderFee, tax, grand,
		instructions, 28+s.rng.Intn(20), 45+s.rng.Intn(20))

	// items
	for _, it := range items {
		if err := s.exec(`
			INSERT INTO order_items
			(order_id, menu_item_id, name_snapshot, is_veg_snapshot, quantity, unit_price_paise, item_total_paise)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			orderID, it.menuItemID, it.name, it.veg, it.qty, it.unit, it.total); err != nil {
			return err
		}
	}

	// delivery address snapshot from the customer's default address
	if _, err := s.tx.Exec(s.ctx, `
		INSERT INTO order_addresses
		(order_id, recipient_name, recipient_phone, line1, line2, landmark, locality, city, state, postal_code, lat, lng, instructions, zone_id)
		SELECT $1, recipient_name, recipient_phone, line1, line2, landmark, locality, city, state, postal_code, lat, lng, delivery_instructions,
			(SELECT id FROM service_zones ORDER BY random() LIMIT 1)
		FROM customer_addresses WHERE user_id = $2 AND is_default AND deleted_at IS NULL LIMIT 1`,
		orderID, customerID); err != nil {
		return err
	}

	if err := s.seedPaymentLifecycle(orderID, grand, status, placedAt, seq); err != nil { // orderID string, grand int64
		return err
	}
	if err := s.seedOrderEvents(orderID, status, placedAt, customerID); err != nil {
		return err
	}
	if err := s.seedDelivery(orderID, status, placedAt, outletID, customerID); err != nil {
		return err
	}
	if status == "delivered" && s.rng.Intn(10) < 6 {
		if err := s.seedReview(orderID, customerID, outletID); err != nil {
			return err
		}
	}
	if couponID != nil {
		if err := s.exec(`
			INSERT INTO coupon_redemptions (coupon_id, user_id, order_id, discount_paise)
			VALUES ($1,$2,$3,$4)`, couponID, customerID, orderID, couponDiscount); err != nil {
			return err
		}
		if _, err := s.tx.Exec(s.ctx,
			`UPDATE coupons SET redeemed_count = redeemed_count + 1 WHERE id = $1`, couponID); err != nil {
			return err
		}
	}
	return nil
}

func isLive(status string) bool {
	switch status {
	case "restaurant_confirmed", "preparing", "rider_assigned", "on_the_way", "ready_for_pickup", "placed":
		return true
	}
	return false
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

type orderLine struct {
	menuItemID string
	name       string
	veg        bool
	tax        string
	unit       int
	qty        int
	total      int
}

func (s *seeder) pickOrderItems(outletID string, n int) []orderLine {
	ids := s.itemIDs[outletID]
	if len(ids) == 0 {
		return nil
	}
	var out []orderLine
	seen := map[string]bool{}
	for len(out) < n && len(seen) < len(ids) {
		id := ids[s.rng.Intn(len(ids))]
		if seen[id] {
			continue
		}
		seen[id] = true
		var name string
		var veg bool
		var price int
		var tax string
		if err := s.tx.QueryRow(s.ctx,
			`SELECT name, is_veg, price_paise, tax_category FROM menu_items WHERE id=$1 AND is_available AND deleted_at IS NULL`,
			id).Scan(&name, &veg, &price, &tax); err != nil {
			continue // item unavailable in this outlet's pool
		}
		qty := 1 + s.rng.Intn(2)
		out = append(out, orderLine{
			menuItemID: id, name: name, veg: veg, tax: tax,
			unit: price, qty: qty, total: price * qty,
		})
	}
	return out
}
