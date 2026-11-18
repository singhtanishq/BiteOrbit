package seed

import (
	"fmt"
	"time"
)

// seedPaymentLifecycle creates payments (mock captured or COD) and refunds
// consistent with the final order status.
func (s *seeder) seedPaymentLifecycle(orderID string, grand int64, status string, placedAt time.Time, seq int) error {
	method := "mock_upi"
	if seq%7 == 3 {
		method = "cod"
	}
	providerStatus := "captured"
	if method == "cod" {
		switch status {
		case "delivered":
			providerStatus = "cod_collected"
		case "rejected", "cancelled":
			providerStatus = "cancelled"
		default:
			providerStatus = "cod_due"
		}
	} else {
		switch status {
		case "rejected", "cancelled":
			providerStatus = "refunded"
		case "delivery_failed":
			providerStatus = "refunded"
		default:
			providerStatus = "captured"
		}
	}

	var capturedAt any
	if providerStatus == "captured" || providerStatus == "cod_collected" {
		capturedAt = placedAt
	}
	paymentID := s.mustInsert(`
		INSERT INTO payments (order_id, provider, method, status, amount_paise, currency, provider_reference, captured_at, created_at)
		VALUES ($1,'mock',$2,$3,$4,'INR',$5,$6,$7) RETURNING id`,
		orderID, method, providerStatus, grand,
		fmt.Sprintf("mockpay_BO_%d", seq),
		capturedAt,
		placedAt)

	if _, err := s.tx.Exec(s.ctx, `
		INSERT INTO payment_attempts (payment_id, attempt_no, status, provider_reference, detail, created_at)
		VALUES ($1,1,$2,$3,'{"simulated":true}'::jsonb,$4)`,
		paymentID, providerStatus, fmt.Sprintf("mockpay_BO_%d", seq), placedAt); err != nil {
		return err
	}

	// refunds where the status path implies money movement back to the customer
	if providerStatus == "refunded" {
		reason := map[string]string{
			"rejected":        "Order rejected by restaurant",
			"cancelled":       "Order cancelled",
			"delivery_failed": "Delivery could not be completed",
		}[status]
		if err := s.exec(`
			INSERT INTO refunds (payment_id, order_id, amount_paise, status, reason, provider_reference, initiated_by, created_at, completed_at)
			VALUES ($1,$2,$3,'processed',$4,$5,'system',$6,$6::timestamptz + interval '2 hours')`,
			paymentID, orderID, grand, reason,
			fmt.Sprintf("mockrefund_BO_%d", seq), placedAt); err != nil {
			return err
		}
		// A pending refund example for one cancelled order in every ~13.
		if seq%13 == 5 {
			if _, err := s.tx.Exec(s.ctx, `
				UPDATE refunds SET status='pending', completed_at=NULL WHERE order_id=$1`, orderID); err != nil {
				return err
			}
		}
	}
	return nil
}

// seedOrderEvents writes the append-only history matching the final status.
func (s *seeder) seedOrderEvents(orderID, status string, placedAt time.Time, customerID string) error {
	type ev struct{ from, to, actor, reason string }
	var chain []ev
	switch status {
	case "delivered":
		chain = []ev{{"", "placed", "customer", ""}, {"placed", "restaurant_confirmed", "restaurant", ""},
			{"restaurant_confirmed", "preparing", "restaurant", ""},
			{"preparing", "ready_for_pickup", "restaurant", ""},
			{"ready_for_pickup", "rider_assigned", "dispatch", ""},
			{"rider_assigned", "picked_up", "rider", ""},
			{"picked_up", "on_the_way", "rider", ""},
			{"on_the_way", "delivered", "rider", ""}}
	case "rejected":
		chain = []ev{{"", "placed", "customer", ""}, {"placed", "rejected", "restaurant", "Outlet closed earlier than expected (demo)"}}
	case "cancelled":
		chain = []ev{{"", "placed", "customer", ""}, {"placed", "restaurant_confirmed", "restaurant", ""},
			{"restaurant_confirmed", "cancel_requested", "customer", "Changed my mind"},
			{"cancel_requested", "cancelled", "system", "Cancellation approved within policy window"}}
	case "delivery_failed":
		chain = []ev{{"", "placed", "customer", ""}, {"placed", "restaurant_confirmed", "restaurant", ""},
			{"restaurant_confirmed", "preparing", "restaurant", ""},
			{"preparing", "ready_for_pickup", "restaurant", ""},
			{"ready_for_pickup", "rider_assigned", "dispatch", ""},
			{"rider_assigned", "picked_up", "rider", ""},
			{"picked_up", "on_the_way", "rider", ""},
			{"on_the_way", "delivery_failed", "rider", "Customer unreachable (demo)"}}
	case "restaurant_confirmed":
		chain = []ev{{"", "placed", "customer", ""}, {"placed", "restaurant_confirmed", "restaurant", ""}}
	case "preparing":
		chain = []ev{{"", "placed", "customer", ""}, {"placed", "restaurant_confirmed", "restaurant", ""},
			{"restaurant_confirmed", "preparing", "restaurant", ""}}
	case "rider_assigned":
		chain = []ev{{"", "placed", "customer", ""}, {"placed", "restaurant_confirmed", "restaurant", ""},
			{"restaurant_confirmed", "preparing", "restaurant", ""},
			{"preparing", "ready_for_pickup", "restaurant", ""},
			{"ready_for_pickup", "rider_assigned", "dispatch", ""}}
	case "on_the_way":
		chain = []ev{{"", "placed", "customer", ""}, {"placed", "restaurant_confirmed", "restaurant", ""},
			{"restaurant_confirmed", "preparing", "restaurant", ""},
			{"preparing", "ready_for_pickup", "restaurant", ""},
			{"ready_for_pickup", "rider_assigned", "dispatch", ""},
			{"rider_assigned", "picked_up", "rider", ""},
			{"picked_up", "on_the_way", "rider", ""}}
	default: // placed / pending_payment etc.
		chain = []ev{{"", "placed", "customer", ""}}
	}

	minutes := 0
	for _, e := range chain {
		minutes += 4 + s.rng.Intn(9)
		at := placedAt.Add(time.Duration(minutes) * time.Minute)
		if at.After(time.Now()) {
			at = time.Now().Add(-time.Duration(s.rng.Intn(30)) * time.Minute)
		}
		var actorID any
		if e.actor == "customer" {
			actorID = customerID
		}
		if err := s.exec(`
			INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id, reason, created_at)
			VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, NULLIF($7,''), $8)`,
			orderID, "state_changed", e.from, e.to, e.actor, actorID, e.reason, at); err != nil {
			return err
		}
	}

	// timestamp columns on the order itself
	if _, err := s.tx.Exec(s.ctx, `
		UPDATE orders SET
			confirmed_at = CASE WHEN $2::text IN ('restaurant_confirmed','preparing','ready_for_pickup','rider_assigned','picked_up','on_the_way','delivered') THEN $3::timestamptz END,
			preparing_at = CASE WHEN $2::text IN ('preparing','ready_for_pickup','rider_assigned','picked_up','on_the_way','delivered') THEN $3::timestamptz + interval '5 minutes' END,
			ready_at = CASE WHEN $2::text IN ('ready_for_pickup','rider_assigned','picked_up','on_the_way','delivered') THEN $3::timestamptz + interval '15 minutes' END,
			delivered_at = CASE WHEN $2::text = 'delivered' THEN $3::timestamptz + interval '35 minutes' END,
			closed_at = CASE WHEN $2::text IN ('delivered','rejected','cancelled','delivery_failed') THEN $3::timestamptz + interval '40 minutes' END
		WHERE id = $1`, orderID, status, placedAt); err != nil {
		return err
	}
	return nil
}

// seedDelivery creates assignments + earnings for orders that reached a rider.
func (s *seeder) seedDelivery(orderID, status string, placedAt time.Time, outletID, customerID string) error {
	needsRider := map[string]bool{
		"rider_assigned": true, "picked_up": true, "on_the_way": true, "delivered": true, "delivery_failed": true,
	}[status]
	if !needsRider {
		return nil
	}
	// pick a rider (any seeded, approved rider)
	var riderID, riderUserID string
	err := s.tx.QueryRow(s.ctx, `
		SELECT dp.id, dp.user_id FROM delivery_partners dp
		JOIN user_roles ur ON ur.user_id = dp.user_id AND ur.role = 'rider'
		WHERE dp.status = 'approved' ORDER BY dp.created_at LIMIT 1 OFFSET $1`,
		s.rng.Intn(6)).Scan(&riderID, &riderUserID)
	if err != nil {
		return nil // no riders seeded yet
	}

	deliveryStatus := map[string]string{
		"rider_assigned": "assigned", "picked_up": "picked_up", "on_the_way": "on_the_way",
		"delivered": "delivered", "delivery_failed": "delivery_failed",
	}[status]
	base := int64(3200 + s.rng.Intn(1800))
	incentive := int64(0)
	if s.rng.Intn(4) == 0 {
		incentive = 2000
	}
	var acceptedAt any
	if deliveryStatus != "assigned" {
		acceptedAt = placedAt.Add(18 * time.Minute)
	}
	assignmentID := s.mustInsert(`
		INSERT INTO delivery_assignments
		(order_id, rider_id, status, offered_at, accepted_at, expires_at, distance_km, assigned_by, payout_base_paise, payout_incentive_paise, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'dispatch',$8,$9,$4) RETURNING id`,
		orderID, riderID, deliveryStatus, placedAt.Add(16*time.Minute),
		acceptedAt,
		placedAt.Add(16*time.Minute+10*time.Minute),
		float64(1+s.rng.Intn(50))/10, base, incentive)

	// assignment event trail
	for _, st := range []string{"assigned", "accepted", "arrived_at_restaurant", "picked_up", "delivered"} {
		if err := s.exec(`
			INSERT INTO delivery_events (assignment_id, event_type, actor_type, actor_id, created_at)
			VALUES ($1,$2,'rider',$3,$4)`,
			assignmentID, st, riderUserID, placedAt.Add(time.Duration(18+len(st))*time.Minute)); err != nil {
			return err
		}
		if st == deliveryStatus || (deliveryStatus == "delivery_failed" && st == "picked_up") {
			break
		}
	}

	// earnings rows only for completed deliveries
	if deliveryStatus == "delivered" {
		tip := ternary(s.rng.Intn(5) == 0, int64(2000+s.rng.Intn(3000)), int64(0))
		if err := s.exec(`
			INSERT INTO rider_earnings (rider_id, assignment_id, base_pay_paise, incentive_paise, tip_paise, total_paise, status)
			VALUES ($1,$2,$3,$4,$5,$6,'payable')`,
			riderID, assignmentID, base, ternary(s.rng.Intn(6) == 0, int64(1500), int64(0)), tip,
			base+tip); err != nil {
			return err
		}
	}
	return nil
}

// seedReview adds a synthetic review for a delivered order.
func (s *seeder) seedReview(orderID, customerID, outletID string) error {
	rating := pickInt(s.rng, 4, 4, 5, 5, 5, 3)
	comments := []string{
		"Hot on arrival, packaging was excellent.",
		"Tastes just like the demo description promised!",
		"Quick delivery, well packed.",
		"Good food, could be spicier.",
		"Loved the biryani — will reorder.",
	}
	age := fmt.Sprintf("%d days", s.rng.Intn(10)+1)
	if _, err := s.tx.Exec(s.ctx, `
		INSERT INTO reviews (order_id, customer_id, outlet_id, rating, delivery_rating, comment, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,'published', now() - $7::interval)`,
		orderID, customerID, outletID, rating, rating,
		pick(s.rng, comments...), age); err != nil {
		return err
	}
	return nil
}

var _ = fmt.Sprintf
var _ = time.Now
