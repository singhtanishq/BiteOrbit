package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"testing"
)

type testAddress struct {
	ID string `json:"id"`
}

func createTestAddress(t *testing.T, jar interface {
	// using env.Do with the jar directly
}, email string) string {
	t.Helper()
	return ""
}

// setupCheckout prepares a signed-in customer (register starts a session on
// the returned jar) with an address and a menu item id.
func setupCheckout(t *testing.T, email string) (*cookiejar.Jar, string, string) {
	t.Helper()
	jar := env.NewJar()
	env.RegisterCustomerWithJar(t, jar, email)
	status, res := env.Do(t, jar, "POST", "/api/v1/me/addresses", map[string]any{
		"label": "home", "recipient_name": "Checkout User", "recipient_phone": "9876543210",
		"line1": "2 Lakeview Avenue", "locality": "Lakeview Enclave", "city": "Bitepur",
		"state": "Telengana Heights", "postal_code": "560021",
	})
	if status != http.StatusCreated {
		t.Fatalf("address create = %d", status)
	}
	var addr testAddress
	_ = json.Unmarshal(res.Data, &addr)
	itemID, _, _ := findMenuItemByName(t, "Ghee Roast Dosa")
	return jar, addr.ID, itemID
}

func TestCheckoutQuoteBreakdown(t *testing.T) {
	email := "checkout-quote@example.test"
	jar, addressID, itemID := setupCheckout(t, email)

	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod", "tip_paise": 2000,
	})
	if status != http.StatusOK {
		t.Fatalf("quote = %d (%v)", status, res.Error)
	}
	var q struct {
		QuoteID   string `json:"quote_id"`
		ExpiresAt string `json:"expires_at"`
		Breakdown struct {
			ItemsSubtotalPaise int64            `json:"items_subtotal_paise"`
			DeliveryFeePaise   int64            `json:"delivery_fee_paise"`
			PlatformFeePaise   int64            `json:"platform_fee_paise"`
			PackagingFeePaise  int64            `json:"packaging_fee_paise"`
			TipPaise           int64            `json:"tip_paise"`
			TaxPaise           int64            `json:"tax_paise"`
			TaxLines           []map[string]any `json:"tax_lines"`
			GrandTotalPaise    int64            `json:"grand_total_paise"`
		} `json:"breakdown"`
		ETAMinMinutes    int  `json:"eta_min_minutes"`
		ETAMaxMinutes    int  `json:"eta_max_minutes"`
		PaymentSimulated bool `json:"payment_simulated"`
	}
	if err := json.Unmarshal(res.Data, &q); err != nil {
		t.Fatalf("quote decode: %v", err)
	}
	if q.QuoteID == "" || q.ExpiresAt == "" {
		t.Fatal("quote id/expiry missing")
	}
	if q.Breakdown.ItemsSubtotalPaise == 0 || q.Breakdown.GrandTotalPaise == 0 {
		t.Fatalf("breakdown incomplete: %+v", q.Breakdown)
	}
	if q.Breakdown.TipPaise != 2000 {
		t.Fatalf("tip not honoured: %d", q.Breakdown.TipPaise)
	}
	// subtotal = unit*2; grand = subtotal + fees + tip + tax, all server-computed
	b := q.Breakdown
	sum := b.ItemsSubtotalPaise - 0 + b.DeliveryFeePaise + b.PlatformFeePaise +
		b.PackagingFeePaise + b.TipPaise + b.TaxPaise
	if b.GrandTotalPaise != sum {
		t.Fatalf("grand total ≠ component sum: %d vs %d", b.GrandTotalPaise, sum)
	}
	if len(b.TaxLines) == 0 {
		t.Fatal("tax lines must be disclosed")
	}
	if q.PaymentSimulated {
		t.Fatal("COD must NOT be marked as a simulated online payment")
	}
}

func TestCheckoutPlaceCODAndIdempotency(t *testing.T) {
	email := "checkout-place@example.test"
	jar, addressID, itemID := setupCheckout(t, email)

	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod",
	})
	if status != http.StatusOK {
		t.Fatalf("quote = %d", status)
	}
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.Unmarshal(res.Data, &q)

	// place with an idempotency key
	key := "test-key-" + email
	envDoWithKey := func() (int, map[string]any) {
		status, res := env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
			map[string]any{"quote_id": q.QuoteID}, map[string]string{"Idempotency-Key": key})
		var data map[string]any
		_ = json.Unmarshal(res.Data, &data)
		return status, data
	}

	status, placed := envDoWithKey()
	if status != http.StatusOK {
		t.Fatalf("place = %d (%v)", status, placed)
	}
	if placed["status"] != "placed" {
		t.Fatalf("COD order status = %v, want placed", placed["status"])
	}
	if placed["payment_status"] != "cod_due" {
		t.Fatalf("COD payment status = %v, want cod_due", placed["payment_status"])
	}
	reference := placed["reference"].(string)

	// duplicate submission with the same key must NOT create a second order
	status2, placed2 := envDoWithKey()
	if status2 != http.StatusOK || placed2["reference"] != reference {
		t.Fatalf("idempotent replay mismatch: %d %v vs %v", status2, placed2["reference"], reference)
	}

	// database must contain exactly one order for this customer
	var count int
	if err := env.Pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM orders WHERE customer_id = (SELECT id FROM users WHERE email = $1)`,
		email).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("duplicate order created! count = %d", count)
	}

	// cart converted
	cartStatus, cartRes := env.Do(t, jar, "GET", "/api/v1/cart", nil)
	if cartStatus != http.StatusOK {
		t.Fatalf("cart get = %d", cartStatus)
	}
	if s := string(cartRes.Data); s != "" && s != "null" {
		t.Fatalf("cart should be gone after conversion, got %s", s)
	}
}

func TestCheckoutStaleQuoteRejected(t *testing.T) {
	email := "checkout-stale@example.test"
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

	// mutate the cart AFTER the quote
	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 1,
	})

	status, res = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "stale-" + email})
	if status != http.StatusConflict {
		t.Fatalf("stale place = %d, want 409 (%v)", status, res.Error)
	}
}

func TestCheckoutExpiredQuoteRejected(t *testing.T) {
	email := "checkout-expired@example.test"
	jar, addressID, itemID := setupCheckout(t, email)

	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod",
	})
	if status != http.StatusOK {
		t.Fatalf("quote = %d (%v)", status, res.Error)
	}
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.Unmarshal(res.Data, &q)

	// force expiry
	if _, err := env.Pool.Exec(t.Context(),
		`UPDATE pricing_quotes SET expires_at = now() - interval '1 minute' WHERE id = $1`,
		q.QuoteID); err != nil {
		t.Fatalf("expire quote: %v", err)
	}

	status, res = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "expired-" + email})
	if status != http.StatusConflict {
		t.Fatalf("expired place = %d, want 409 (%v)", status, res.Error)
	}
}

func TestCheckoutForeignQuoteRejected(t *testing.T) {
	email := "checkout-owner@example.test"
	jar, addressID, itemID := setupCheckout(t, email)

	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod",
	})
	if status != http.StatusOK {
		t.Fatalf("owner quote = %d (%v)", status, res.Error)
	}
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.Unmarshal(res.Data, &q)

	// a different customer tries to place this quote
	env.RegisterCustomer(t, "checkout-thief@example.test")
	thiefJar := env.NewJar()
	_, _ = env.Do(t, thiefJar, "POST", "/api/v1/auth/login", map[string]any{
		"email": "checkout-thief@example.test", "password": "Passw0rdLocal!",
	})
	status, res = env.DoWithHeader(t, thiefJar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "thief-" + email})
	if status != http.StatusNotFound && status != http.StatusForbidden {
		t.Fatalf("foreign quote place = %d, want 404/403 (%v)", status, res.Error)
	}
}

func TestCheckoutCouponFlows(t *testing.T) {
	email := "checkout-coupon@example.test"
	jar, addressID, itemID := setupCheckout(t, email)

	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})

	// valid coupon: ORBIT50 (fixed ₹50 off above ₹299); dose up past the minimum
	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 3,
	})
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/coupon", map[string]any{"code": "ORBIT50"})
	if status != http.StatusOK {
		t.Fatalf("coupon validate = %d", status)
	}
	var vc struct {
		Valid         bool   `json:"valid"`
		DiscountPaise int64  `json:"discount_paise"`
		Reason        string `json:"reason"`
	}
	_ = json.Unmarshal(res.Data, &vc)
	if !vc.Valid || vc.DiscountPaise != 5000 {
		t.Fatalf("ORBIT50 should give ₹50 off: %+v", vc)
	}

	// quote with coupon: discount appears in the breakdown and grand total
	status, res = env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod", "coupon_code": "ORBIT50",
	})
	if status != http.StatusOK {
		t.Fatalf("quote with coupon = %d (%v)", status, res.Error)
	}
	var q struct {
		QuoteID   string `json:"quote_id"`
		Breakdown struct {
			CouponDiscountPaise int64 `json:"coupon_discount_paise"`
			GrandTotalPaise     int64 `json:"grand_total_paise"`
		} `json:"breakdown"`
	}
	_ = json.Unmarshal(res.Data, &q)
	if q.Breakdown.CouponDiscountPaise != 5000 {
		t.Fatalf("quote discount = %d", q.Breakdown.CouponDiscountPaise)
	}

	// place the order → redemption recorded
	status, res = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "coupon-" + email})
	if status != http.StatusOK {
		t.Fatalf("place with coupon = %d (%v)", status, res.Error)
	}
	var placed struct {
		OrderID string `json:"order_id"`
	}
	_ = json.Unmarshal(res.Data, &placed)

	var redemptions int
	_ = env.Pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM coupon_redemptions WHERE coupon_id = (SELECT id FROM coupons WHERE code='ORBIT50')
		AND user_id = (SELECT id FROM users WHERE email = $1)`, email).Scan(&redemptions)
	if redemptions != 1 {
		t.Fatalf("redemptions = %d, want 1", redemptions)
	}
	_ = placed

	// second use within per-user limit (ORBIT50 allows 2; a third is rejected)
	for i := 0; i < 1; i++ {
		_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
			"menu_item_id": itemID, "quantity": 3,
		})
		status, res = env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
			"address_id": addressID, "payment_method": "cod", "coupon_code": "ORBIT50",
		})
		if status != http.StatusOK {
			t.Fatalf("requote %d = %d (%v)", i, status, res.Error)
		}
		_ = json.Unmarshal(res.Data, &q)
		status, res = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
			map[string]any{"quote_id": q.QuoteID},
			map[string]string{"Idempotency-Key": fmt.Sprintf("coupon-%s-%d", email, i)})
		if status != http.StatusOK {
			t.Fatalf("re-place %d = %d (%v)", i, status, res.Error)
		}
	}
	// third redemption must fail validation
	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 3,
	})
	status, res = env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod", "coupon_code": "ORBIT50",
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("third ORBIT50 quote = %d, want 422 (user limit)", status)
	}
}

func TestCheckoutExpiredCouponRejected(t *testing.T) {
	email := "checkout-expired-coupon@example.test"
	jar, _, itemID := setupCheckout(t, email)
	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 4,
	})
	// SALENI is seeded as expired
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/coupon", map[string]any{"code": "SALENI"})
	var vc struct {
		Valid  bool   `json:"valid"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(res.Data, &vc)
	if status != http.StatusOK || vc.Valid || vc.Reason != "expired" {
		t.Fatalf("expired coupon accepted: %d %+v", status, vc)
	}
}

func TestCheckoutNewUserCouponRejectsExistingCustomer(t *testing.T) {
	email := "checkout-newuser@example.test"
	jar, addressID, itemID := setupCheckout(t, email)

	// place a normal order first
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
	status, _ = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "newuser-first-" + email})
	if status != http.StatusOK {
		t.Fatalf("first place = %d", status)
	}

	// NEWBITE (new users only) must now be rejected
	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res = env.Do(t, jar, "POST", "/api/v1/checkout/coupon", map[string]any{"code": "NEWBITE"})
	var vc struct {
		Valid  bool   `json:"valid"`
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(res.Data, &vc)
	if status != http.StatusOK || vc.Valid || vc.Reason != "new_users_only" {
		t.Fatalf("NEWBITE accepted for existing customer: %d %+v", status, vc)
	}
}

func TestCheckoutCouponGlobalLimitRace(t *testing.T) {
	// create a coupon with total limit 1, then race two placements
	emailA := "race-a@example.test"
	emailB := "race-b@example.test"
	_, _ = env.Pool.Exec(t.Context(), `
		INSERT INTO coupons (code, title, discount_type, fixed_paise, min_order_paise,
			starts_at, is_active, usage_limit_per_user, total_redemption_limit)
		VALUES ('RACE1', 'Race test (demo)', 'fixed', 2000, 0, now() - interval '1 day', true, 1, 1)`)

	itemID, _, _ := findMenuItemByName(t, "Masala Dosa")

	var wg sync.WaitGroup
	results := make([]int, 2)
	panics := make([]error, 2)
	// pre-register both customers from the MAIN goroutine (t is not goroutine-safe)
	env.RegisterCustomer(t, emailA)
	env.RegisterCustomer(t, emailB)
	for i, email := range []string{emailA, emailB} {
		wg.Add(1)
		go func(idx int, mail string) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					panics[idx] = fmt.Errorf("goroutine panic: %v", rec)
				}
			}()
			jar := env.NewJar()
			_, _ = env.Do(t, jar, "POST", "/api/v1/auth/login", map[string]any{
				"email": mail, "password": "Passw0rdLocal!",
			})
			status, res := env.Do(t, jar, "POST", "/api/v1/me/addresses", map[string]any{
				"label": "home", "recipient_name": "R", "recipient_phone": "9876543210",
				"line1": "3 Meadow Way", "locality": "Silicon Meadow", "city": "Bitepur",
				"state": "Telengana Heights", "postal_code": "560031",
			})
			if status != http.StatusCreated {
				results[idx] = status
				return
			}
			var addr testAddress
			_ = json.Unmarshal(res.Data, &addr)
			_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
				"menu_item_id": itemID, "quantity": 3,
			})
			status, res = env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
				"address_id": addr.ID, "payment_method": "cod", "coupon_code": "RACE1",
			})
			if status != http.StatusOK {
				results[idx] = status
				return
			}
			var q struct {
				QuoteID string `json:"quote_id"`
			}
			_ = json.Unmarshal(res.Data, &q)
			status, _ = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
				map[string]any{"quote_id": q.QuoteID},
				map[string]string{"Idempotency-Key": "race-" + mail})
			results[idx] = status
		}(i, email)
	}
	wg.Wait()

	for idx, p := range panics {
		if p != nil {
			t.Fatalf("race goroutine %d failed: %v", idx, p)
		}
	}

	oks := 0
	for _, s := range results {
		if s == http.StatusOK {
			oks++
		}
	}
	if oks != 1 {
		t.Fatalf("global-limit race: expected exactly 1 success, got %d (statuses %v)", oks, results)
	}
	var redeemed int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT redeemed_count FROM coupons WHERE code='RACE1'`).Scan(&redeemed)
	if redeemed != 1 {
		t.Fatalf("redeemed_count = %d, want exactly 1", redeemed)
	}
}
