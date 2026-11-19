package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

func TestReviewsLifecycle(t *testing.T) {
	// drive an order to delivered with COD
	jarC, orderID, _, _ := driveOrderToReady(t)
	triggerDispatch(t, orderID)
	riderEmail := assignedRiderEmail(t, orderID)
	riderJar := loginRider(t, riderEmail)
	assignmentID := assignmentIDFor(t, orderID)
	_, _ = env.Do(t, riderJar, "POST", "/api/v1/courier/offers/"+assignmentID+"/accept", map[string]any{})
	for _, st := range []string{"arriving_at_restaurant", "arrived_at_restaurant", "picked_up", "on_the_way"} {
		_, _ = env.Do(t, riderJar, "POST", "/api/v1/courier/assignments/"+assignmentID+"/state", map[string]any{"state": st})
	}
	otp := otpFromAssignment(t, assignmentID)
	status, _ := env.Do(t, riderJar, "POST", "/api/v1/courier/assignments/"+assignmentID+"/state",
		map[string]any{"state": "delivered", "otp": otp})
	if status != http.StatusOK {
		t.Fatalf("delivery = %d", status)
	}

	// review the delivered order
	status, res := env.Do(t, jarC, "POST", "/api/v1/reviews", map[string]any{
		"order_id": orderID, "rating": 5, "delivery_rating": 4, "comment": "Great biryani, hot on arrival!",
	})
	if status != http.StatusCreated {
		t.Fatalf("review = %d (%v)", status, res.Error)
	}
	var created struct {
		ID       string `json:"id"`
		OutletID string `json:"outlet_id"`
	}
	_ = json.Unmarshal(res.Data, &created)

	// duplicate review rejected
	status, res = env.Do(t, jarC, "POST", "/api/v1/reviews", map[string]any{
		"order_id": orderID, "rating": 1,
	})
	if status != http.StatusConflict {
		t.Fatalf("duplicate review = %d, want 409", status)
	}

	// reviews appear on the outlet page
	status, res = env.Do(t, env.NewJar(), "GET", "/api/v1/restaurants/"+created.OutletID+"/reviews", nil)
	if status != http.StatusOK {
		t.Fatalf("outlet reviews = %d", status)
	}
	var list []map[string]any
	_ = json.Unmarshal(res.Data, &list)
	found := false
	for _, rv := range list {
		if rv["comment"] == "Great biryani, hot on arrival!" {
			found = true
		}
	}
	if !found {
		t.Fatal("new review missing from outlet reviews")
	}

	// report for moderation (any signed-in user)
	reporterJar := env.NewJar()
	env.RegisterCustomerWithJar(t, reporterJar, "review-reporter@example.test")
	status, _ = env.Do(t, reporterJar, "POST", "/api/v1/reviews/"+created.ID+"/report",
		map[string]any{"reason": "Looks like spam (demo)"})
	if status != http.StatusCreated {
		t.Fatalf("report = %d", status)
	}

	// rating 6 rejected
	status, _ = env.Do(t, jarC, "POST", "/api/v1/reviews", map[string]any{
		"order_id": orderID, "rating": 6,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("rating 6 accepted (%d)", status)
	}
}

func assignedRiderEmail(t *testing.T, orderID string) string {
	t.Helper()
	var email string
	if err := env.Pool.QueryRow(t.Context(), `
		SELECT u.email FROM delivery_assignments da
		JOIN delivery_partners dp ON dp.id = da.rider_id
		JOIN users u ON u.id = dp.user_id
		WHERE da.order_id = $1`, orderID).Scan(&email); err != nil {
		t.Fatalf("assigned rider: %v", err)
	}
	return email
}

func assignmentIDFor(t *testing.T, orderID string) string {
	t.Helper()
	var id string
	if err := env.Pool.QueryRow(t.Context(),
		`SELECT id FROM delivery_assignments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1`,
		orderID).Scan(&id); err != nil {
		t.Fatalf("assignment: %v", err)
	}
	return id
}

func TestFavouritesFlow(t *testing.T) {
	email := "fav-user@example.test"
	env.RegisterCustomer(t, email)
	jar := env.NewJar()
	loginViaJarEmail(t, jar, email)

	// favourite the first outlet
	status, res := env.Do(t, jar, "GET", "/api/v1/discovery/restaurants?limit=1", nil)
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	outletID := outlets[0]["id"].(string)

	status, _ = env.Do(t, jar, "POST", "/api/v1/me/favourites/"+outletID, nil)
	if status != http.StatusCreated {
		t.Fatalf("favourite = %d", status)
	}
	// idempotent-ish: second add is fine
	status, _ = env.Do(t, jar, "POST", "/api/v1/me/favourites/"+outletID, nil)
	if status != http.StatusCreated {
		t.Fatalf("re-favourite = %d", status)
	}

	status, res = env.Do(t, jar, "GET", "/api/v1/me/favourites", nil)
	var favs []map[string]any
	_ = json.Unmarshal(res.Data, &favs)
	if len(favs) != 1 || favs[0]["id"] != outletID {
		t.Fatalf("favourites = %v", favs)
	}

	// unfavourite
	status, _ = env.Do(t, jar, "DELETE", "/api/v1/me/favourites/"+outletID, nil)
	if status != http.StatusOK {
		t.Fatalf("unfavourite = %d", status)
	}
	status, res = env.Do(t, jar, "GET", "/api/v1/me/favourites", nil)
	_ = json.Unmarshal(res.Data, &favs)
	if len(favs) != 0 {
		t.Fatalf("favourites after removal = %d", len(favs))
	}
}

func loginViaJarEmail(t *testing.T, jar *cookiejar.Jar, email string) {
	t.Helper()
	_, _ = env.Do(t, jar, "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Passw0rdLocal!",
	})
}

func TestReorderRespectsCurrentPricesAndAvailability(t *testing.T) {
	email := "reorder-user@example.test"
	jar, addressID, itemID := setupCheckout(t, email)
	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod",
	})
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.Unmarshal(res.Data, &q)
	status, res = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "reorder-" + email})
	var placed struct {
		OrderID string `json:"order_id"`
	}
	_ = json.Unmarshal(res.Data, &placed)

	// make the item unavailable after delivery... (order is placed; mark
	// it delivered directly as a demo shortcut)
	_, _ = env.Pool.Exec(t.Context(),
		`UPDATE orders SET status='delivered', delivered_at=now(), closed_at=now() WHERE id=$1`, placed.OrderID)
	// and take the item off the menu
	_, _ = env.Pool.Exec(t.Context(),
		`UPDATE menu_items SET is_available=false WHERE id=$1`, itemID)

	status, res = env.Do(t, jar, "POST", "/api/v1/orders/"+placed.OrderID+"/reorder", nil)
	if status != http.StatusCreated {
		t.Fatalf("reorder = %d (%v)", status, res.Error)
	}
	var rr struct {
		Added   int    `json:"added"`
		Skipped int    `json:"skipped"`
		CartID  string `json:"cart_id"`
	}
	_ = json.Unmarshal(res.Data, &rr)
	if rr.Added != 0 || rr.Skipped != 1 {
		t.Fatalf("unavailable item must be skipped: %+v", rr)
	}

	// cart should exist but be empty
	status, res = env.Do(t, jar, "GET", "/api/v1/cart", nil)
	var cart struct {
		Items []any `json:"items"`
	}
	_ = json.Unmarshal(res.Data, &cart)
	if len(cart.Items) != 0 {
		t.Fatalf("unavailable item added to cart: %d", len(cart.Items))
	}

	// restore availability and reorder again → added this time
	_, _ = env.Pool.Exec(t.Context(), `UPDATE menu_items SET is_available=true WHERE id=$1`, itemID)
	status, res = env.Do(t, jar, "POST", "/api/v1/orders/"+placed.OrderID+"/reorder", nil)
	_ = json.Unmarshal(res.Data, &rr)
	if status != http.StatusCreated || rr.Added != 1 {
		t.Fatalf("reorder with available item = %d added %d", status, rr.Added)
	}
}

func TestExportsRequireRoles(t *testing.T) {
	// customer cannot access either export
	cj := env.NewJar()
	env.RegisterCustomerWithJar(t, cj, "export-customer@example.test")
	status, _ := env.Do(t, cj, "GET", "/api/v1/admin/orders/export.csv", nil)
	if status != http.StatusForbidden {
		t.Fatalf("customer admin export = %d, want 403", status)
	}
	status, _ = env.Do(t, cj, "GET", "/api/v1/restaurant-portal/analytics/export.csv?outlet_id=00000000-0000-0000-0000-000000000000", nil)
	if status != http.StatusForbidden {
		t.Fatalf("customer portal export = %d, want 403", status)
	}

	// staff export produces CSV with a header
	sjar := loginAsStaff(t, "manager01@demo.biteorbit.local")
	status, res := env.Do(t, sjar, "GET", "/api/v1/restaurant-portal/outlets", nil)
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	csvRes := env.DoRaw(t, sjar, "GET", "/api/v1/restaurant-portal/analytics/export.csv?outlet_id="+outlets[0]["id"].(string), nil)
	if csvRes.Status != http.StatusOK {
		t.Fatalf("staff export = %d", csvRes.Status)
	}
	if csvRes.ContentType != "text/csv" || csvRes.Body == "" ||
		csvRes.Body[:9] != "day,order" {
		t.Fatalf("unexpected CSV: %q (%s)", csvRes.Body[:min(60, len(csvRes.Body))], csvRes.ContentType)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
