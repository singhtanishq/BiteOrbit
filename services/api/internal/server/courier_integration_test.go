package server_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	"biteorbit/services/api/internal/dispatch"
)

func loginRider(t *testing.T, email string) *cookiejar.Jar {
	t.Helper()
	jar := env.NewJar()
	status, res := env.Do(t, jar, "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Demo@1234",
	})
	if status != http.StatusOK {
		t.Fatalf("rider login = %d (%v)", status, res.Error)
	}
	return jar
}

// driveOrderToReady takes a fresh COD order through accept→prepare→ready.
func driveOrderToReady(t *testing.T) (customerJar *cookiejar.Jar, orderID, outletID, itemID string) {
	t.Helper()
	jar := loginAsStaff(t, "manager01@demo.biteorbit.local")
	status, res := env.Do(t, jar, "GET", "/api/v1/restaurant-portal/outlets", nil)
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	outletID = outlets[0]["id"].(string)
	err := env.Pool.QueryRow(t.Context(), `
		SELECT id FROM menu_items WHERE outlet_id = $1 AND is_available AND deleted_at IS NULL
		ORDER BY price_paise DESC LIMIT 1`, outletID).Scan(&itemID)
	if err != nil {
		t.Skipf("no items: %v", err)
	}

	email := fmt.Sprintf("courier-order-%d@example.test", time.Now().UnixNano())
	customerJar, addressID, _ := setupCheckout(t, email)
	_, _ = env.Do(t, customerJar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res = env.Do(t, customerJar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod",
	})
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.Unmarshal(res.Data, &q)
	status, res = env.DoWithHeader(t, customerJar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "courier-" + email})
	if status != http.StatusOK {
		t.Fatalf("place = %d (%v)", status, res.Error)
	}
	var p struct {
		OrderID string `json:"order_id"`
	}
	_ = json.Unmarshal(res.Data, &p)
	orderID = p.OrderID

	for _, action := range []string{"accept", "prepare", "ready"} {
		st, r2 := env.Do(t, jar, "POST", "/api/v1/restaurant-portal/orders/"+orderID+"/"+action, map[string]any{})
		if st != http.StatusOK {
			t.Fatalf("%s = %d (%v)", action, st, r2.Error)
		}
	}
	return customerJar, orderID, outletID, itemID
}

// triggerDispatch invokes the same dispatch handler the outbox worker runs.
func triggerDispatch(t *testing.T, orderID string) {
	t.Helper()
	h := dispatch.NewHandler(env.Pool, testLogger())
	if err := h.TryAssign(t.Context(), orderID); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
}

// otpFromAssignment brute-forces the 4-digit demo OTP from its stored hash
// (short codespace acceptable for tests only; production uses longer codes).
func otpFromAssignment(t *testing.T, assignmentID string) string {
	t.Helper()
	var hash string
	if err := env.Pool.QueryRow(t.Context(),
		`SELECT pickup_otp_hash FROM delivery_assignments WHERE id = $1`, assignmentID).Scan(&hash); err != nil {
		t.Fatalf("otp hash: %v", err)
	}
	for i := 0; i < 10000; i++ {
		otp := fmt.Sprintf("%04d", i)
		d := sha256.Sum256([]byte(otp))
		if hex.EncodeToString(d[:]) == hash {
			return otp
		}
	}
	t.Fatal("could not brute-force test OTP")
	return ""
}

func TestCourierDispatchAndDelivery(t *testing.T) {
	// every seeded rider goes online (dispatch picks the best candidate)
	var riderEmail string
	err := env.Pool.QueryRow(t.Context(), `
		SELECT u.email FROM delivery_partners dp
		JOIN users u ON u.id = dp.user_id
		WHERE dp.status='approved' AND dp.availability='online'
		ORDER BY dp.created_at LIMIT 1`).Scan(&riderEmail)
	if err != nil {
		t.Skipf("no riders: %v", err)
	}
	riderJar := loginRider(t, riderEmail)

	status, res := env.Do(t, riderJar, "POST", "/api/v1/courier/availability", map[string]any{"online": true})
	if status != http.StatusOK {
		t.Fatalf("availability = %d (%v)", status, res.Error)
	}

	customerJar, orderID, _, _ := driveOrderToReady(t)
	triggerDispatch(t, orderID)

	// the winner rider logs in and sees the offer
	err = env.Pool.QueryRow(t.Context(), `
		SELECT u.email FROM delivery_assignments da
		JOIN delivery_partners dp ON dp.id = da.rider_id
		JOIN users u ON u.id = dp.user_id
		WHERE da.order_id = $1`, orderID).Scan(&riderEmail)
	if err != nil {
		t.Fatalf("no assignment created by dispatch: %v", err)
	}
	riderJar = loginRider(t, riderEmail)
	status, res = env.Do(t, riderJar, "GET", "/api/v1/courier/offers", nil)
	if status != http.StatusOK {
		t.Fatalf("offers = %d (%v)", status, res.Error)
	}
	var offers []map[string]any
	_ = json.Unmarshal(res.Data, &offers)
	found := false
	var assignmentID string
	for _, o := range offers {
		if o["order_id"] == orderID {
			found = true
			assignmentID = o["assignment_id"].(string)
		}
	}
	if !found {
		t.Fatalf("no offer for order %s: %v", orderID, offers)
	}
	// offer hides the drop address until accepted (locality only)
	if _, hasLine := offers[0]["drop_line1"]; hasLine {
		t.Fatal("offer leaks the drop address line")
	}

	// accept
	status, res = env.Do(t, riderJar, "POST", "/api/v1/courier/offers/"+assignmentID+"/accept", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("accept = %d (%v)", status, res.Error)
	}

	// a second rider cannot claim the same assignment
	rider2Jar := loginRider(t, "rider2@demo.biteorbit.local")
	_, _ = env.Do(t, rider2Jar, "POST", "/api/v1/courier/availability", map[string]any{"online": true})
	status, _ = env.Do(t, rider2Jar, "POST", "/api/v1/courier/offers/"+assignmentID+"/accept", map[string]any{})
	if status != http.StatusConflict {
		t.Fatalf("second accept = %d, want 409", status)
	}

	// customer tracking shows rider_assigned
	status, res = env.Do(t, customerJar, "GET", "/api/v1/orders/"+orderID+"/track", nil)
	var track map[string]any
	_ = json.Unmarshal(res.Data, &track)
	if track["status"] != "rider_assigned" {
		t.Fatalf("track status = %v", track["status"])
	}

	// invalid transition rejected
	status, res = env.Do(t, riderJar, "POST", "/api/v1/courier/assignments/"+assignmentID+"/state",
		map[string]any{"state": "delivered"})
	if status != http.StatusConflict {
		t.Fatalf("invalid transition = %d, want 409", status)
	}

	// lifecycle walk
	for _, st := range []string{"arriving_at_restaurant", "arrived_at_restaurant"} {
		status, res = env.Do(t, riderJar, "POST", "/api/v1/courier/assignments/"+assignmentID+"/state", map[string]any{"state": st})
		if status != http.StatusOK {
			t.Fatalf("%s = %d (%v)", st, status, res.Error)
		}
	}
	status, res = env.Do(t, riderJar, "POST", "/api/v1/courier/assignments/"+assignmentID+"/state", map[string]any{"state": "picked_up"})
	if status != http.StatusOK {
		t.Fatalf("picked_up = %d (%v)", status, res.Error)
	}
	status, res = env.Do(t, riderJar, "POST", "/api/v1/courier/assignments/"+assignmentID+"/state", map[string]any{"state": "on_the_way"})
	if status != http.StatusOK {
		t.Fatalf("on_the_way = %d", status)
	}

	// delivery requires the OTP the customer shows (hash-stored server side)
	otp := otpFromAssignment(t, assignmentID)
	status, res = env.Do(t, riderJar, "POST", "/api/v1/courier/assignments/"+assignmentID+"/state",
		map[string]any{"state": "delivered", "otp": "0000"})
	if status != http.StatusBadRequest {
		t.Fatalf("wrong OTP accepted (%d)", status)
	}
	status, res = env.Do(t, riderJar, "POST", "/api/v1/courier/assignments/"+assignmentID+"/state",
		map[string]any{"state": "delivered", "otp": otp})
	if status != http.StatusOK {
		t.Fatalf("delivered = %d (%v)", status, res.Error)
	}

	// final state: order delivered, COD collected, earnings recorded
	status, res = env.Do(t, customerJar, "GET", "/api/v1/orders/"+orderID+"/track", nil)
	_ = json.Unmarshal(res.Data, &track)
	if track["status"] != "delivered" {
		t.Fatalf("final status = %v", track["status"])
	}
	var codStatus string
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT status FROM payments WHERE order_id = $1 LIMIT 1`, orderID).Scan(&codStatus)
	if codStatus != "cod_collected" {
		t.Fatalf("COD payment = %s", codStatus)
	}
	var earnings int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM rider_earnings WHERE assignment_id = $1`, assignmentID).Scan(&earnings)
	if earnings != 1 {
		t.Fatal("earnings row missing after delivery")
	}

	status, res = env.Do(t, riderJar, "GET", "/api/v1/courier/earnings", nil)
	var earn struct {
		Totals struct {
			Deliveries int64 `json:"deliveries"`
			Total      int64 `json:"total_paise"`
		} `json:"totals"`
	}
	_ = json.Unmarshal(res.Data, &earn)
	if earn.Totals.Deliveries < 1 || earn.Totals.Total <= 0 {
		t.Fatalf("earnings empty: %+v", earn)
	}
}

func TestCourierNoRiderQueueHonest(t *testing.T) {
	// take all riders offline, then check the queue reason path
	_, _ = env.Pool.Exec(t.Context(), `UPDATE delivery_partners SET availability='offline'`)
	defer func() {
		_, _ = env.Pool.Exec(t.Context(), `UPDATE delivery_partners SET availability='online'`)
	}()

	_, orderID, _, _ := driveOrderToReady(t)
	triggerDispatch(t, orderID)

	var reason *string
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT rider_queue_reason FROM orders WHERE id = $1`, orderID).Scan(&reason)
	var assignments int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM delivery_assignments WHERE order_id = $1`, orderID).Scan(&assignments)
	if assignments != 0 {
		t.Fatalf("fabricated assignment with no riders online (%d)", assignments)
	}
	if reason == nil || *reason == "" {
		t.Fatal("no honest queue reason recorded")
	}
}

func TestCourierRoleGuard(t *testing.T) {
	// customer must not touch courier routes
	cj := env.NewJar()
	env.RegisterCustomerWithJar(t, cj, "courier-guard@example.test")
	status, _ := env.Do(t, cj, "GET", "/api/v1/courier/offers", nil)
	if status != http.StatusForbidden {
		t.Fatalf("customer on courier offers = %d, want 403", status)
	}
	// anonymous
	status, _ = env.Do(t, env.NewJar(), "GET", "/api/v1/courier/me", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous on courier = %d, want 401", status)
	}
}
