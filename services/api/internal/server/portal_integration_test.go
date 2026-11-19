package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

// loginAsStaff logs in as a seeded restaurant staff account.
func loginAsStaff(t *testing.T, email string) *cookiejar.Jar {
	t.Helper()
	jar := env.NewJar()
	status, res := env.Do(t, jar, "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Demo@1234",
	})
	if status != http.StatusOK {
		t.Fatalf("staff login = %d (%v)", status, res.Error)
	}
	return jar
}

func TestPortalOutletScoping(t *testing.T) {
	jar := loginAsStaff(t, "manager01@demo.biteorbit.local")
	status, res := env.Do(t, jar, "GET", "/api/v1/restaurant-portal/outlets", nil)
	if status != http.StatusOK {
		t.Fatalf("outlets = %d", status)
	}
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	if len(outlets) == 0 {
		t.Fatal("manager has no outlets")
	}
	outletID := outlets[0]["id"].(string)

	// a different staff member cannot see this outlet's data
	jar2 := loginAsStaff(t, "manager02@demo.biteorbit.local")
	status, res = env.Do(t, jar2, "GET", "/api/v1/restaurant-portal/orders?outlet_id="+outletID, nil)
	if status != http.StatusNotFound {
		t.Fatalf("foreign outlet orders = %d, want 404", status)
	}

	// a customer role is rejected outright
	cj := env.NewJar()
	env.RegisterCustomerWithJar(t, cj, "portal-customer@example.test")
	status, _ = env.Do(t, cj, "GET", "/api/v1/restaurant-portal/outlets", nil)
	if status != http.StatusForbidden {
		t.Fatalf("customer on portal = %d, want 403", status)
	}
	_ = res
}

func TestPortalAcceptPrepareReadyFlow(t *testing.T) {
	// staff outlet
	jar := loginAsStaff(t, "manager01@demo.biteorbit.local")
	status, res := env.Do(t, jar, "GET", "/api/v1/restaurant-portal/outlets", nil)
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	outletID := outlets[0]["id"].(string)

	// customer places a COD order at that outlet: find an item of the outlet
	var itemID string
	err := env.Pool.QueryRow(t.Context(), `
		SELECT id FROM menu_items WHERE outlet_id = $1 AND is_available AND deleted_at IS NULL
		ORDER BY price_paise DESC LIMIT 1`, outletID).Scan(&itemID)
	if err != nil {
		t.Skipf("outlet has no items: %v", err)
	}

	custEmail := "portal-order@example.test"
	jarC, addressID, _ := setupCheckout(t, custEmail)
	_, _ = env.Do(t, jarC, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res = env.Do(t, jarC, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "cod",
	})
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.Unmarshal(res.Data, &q)
	status, res = env.DoWithHeader(t, jarC, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "portal-" + custEmail})
	if status != http.StatusOK {
		t.Fatalf("place = %d", status)
	}
	var placed struct {
		OrderID string `json:"order_id"`
	}
	_ = json.Unmarshal(res.Data, &placed)

	// queue shows the order
	status, res = env.Do(t, jar, "GET", "/api/v1/restaurant-portal/orders?outlet_id="+outletID+"&status=placed", nil)
	if status != http.StatusOK {
		t.Fatalf("queue = %d", status)
	}
	var queue []map[string]any
	_ = json.Unmarshal(res.Data, &queue)
	found := false
	for _, o := range queue {
		if o["id"] == placed.OrderID {
			found = true
		}
	}
	if !found {
		t.Fatal("placed order missing from queue")
	}

	// accept
	status, res = env.Do(t, jar, "POST", "/api/v1/restaurant-portal/orders/"+placed.OrderID+"/accept", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("accept = %d (%v)", status, res.Error)
	}

	// double accept must conflict (state guard)
	status, res = env.Do(t, jar, "POST", "/api/v1/restaurant-portal/orders/"+placed.OrderID+"/accept", map[string]any{})
	if status != http.StatusConflict {
		t.Fatalf("double accept = %d, want 409", status)
	}

	// prepare → ready
	status, _ = env.Do(t, jar, "POST", "/api/v1/restaurant-portal/orders/"+placed.OrderID+"/prepare", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("prepare = %d", status)
	}
	status, _ = env.Do(t, jar, "POST", "/api/v1/restaurant-portal/orders/"+placed.OrderID+"/ready", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("ready = %d", status)
	}

	// customer sees preparing → ready transitions
	status, res = env.Do(t, jarC, "GET", "/api/v1/orders/"+placed.OrderID, nil)
	var detail struct {
		Status string `json:"status"`
		Events []struct {
			To string `json:"to_state"`
		} `json:"events"`
	}
	_ = json.Unmarshal(res.Data, &detail)
	if detail.Status != "ready_for_pickup" {
		t.Fatalf("status = %s", detail.Status)
	}
	if len(detail.Events) < 3 {
		t.Fatalf("expected full transition trail, got %d events", len(detail.Events))
	}

	// the restaurant must NOT be able to mark the order delivered (no such route exists —
	// verify that pretending doesn't work by patching through a wrong transition)
	status, _ = env.Do(t, jar, "POST", "/api/v1/restaurant-portal/orders/"+placed.OrderID+"/prepare", map[string]any{})
	if status != http.StatusConflict {
		t.Fatalf("prepare after ready = %d, want 409", status)
	}
}

func TestPortalRejectWithRefund(t *testing.T) {
	jar := loginAsStaff(t, "manager01@demo.biteorbit.local")
	_, res := env.Do(t, jar, "GET", "/api/v1/restaurant-portal/outlets", nil)
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	outletID := outlets[0]["id"].(string)

	var itemID string
	err := env.Pool.QueryRow(t.Context(), `
		SELECT id FROM menu_items WHERE outlet_id = $1 AND is_available AND deleted_at IS NULL LIMIT 1`,
		outletID).Scan(&itemID)
	if err != nil {
		t.Skipf("no items: %v", err)
	}

	// online order + capture
	jarC, orderID, paymentID, _ := setupOnlineOrderAt(t, outletID, itemID)
	_, _ = env.Do(t, jarC, "POST", "/api/v1/payments/"+paymentID+"/simulate", map[string]any{"outcome": "success"})

	// reject with reason
	rejectStatus, rejectRes := env.Do(t, jar, "POST", "/api/v1/restaurant-portal/orders/"+orderID+"/reject",
		map[string]any{"reason": "Outlet closing early (demo)"})
	if rejectStatus != http.StatusOK {
		t.Fatalf("reject = %d (%v)", rejectStatus, rejectRes.Error)
	}

	// payment refunded
	var payStatus string
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&payStatus)
	if payStatus != "refunded" {
		t.Fatalf("payment after rejection = %s, want refunded", payStatus)
	}
	var refunds int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM refunds WHERE order_id = $1 AND status='processed'`, orderID).Scan(&refunds)
	if refunds != 1 {
		t.Fatalf("refund rows = %d", refunds)
	}

	// rejection without a reason fails validation for an accessible order
	_, orderID2, _, _ := setupOnlineOrderAt(t, outletID, itemID)
	status2, _ := env.Do(t, jar, "POST", "/api/v1/restaurant-portal/orders/"+orderID2+"/reject", map[string]any{})
	if status2 != http.StatusBadRequest {
		t.Fatalf("reason-less reject = %d, want 400", status2)
	}
}

// setupOnlineOrderAt places a captured online order at a specific outlet.
func setupOnlineOrderAt(t *testing.T, outletID, itemID string) (*cookiejar.Jar, string, string, string) {
	t.Helper()
	email := fmt.Sprintf("portal-online-%d@example.test", time.Now().UnixNano())
	jar, addressID, _ := setupCheckout(t, email)
	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": "mock_upi",
	})
	if status != http.StatusOK {
		t.Fatalf("quote = %d (%v)", status, res.Error)
	}
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.Unmarshal(res.Data, &q)
	status, res = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "portal-online-" + email})
	if status != http.StatusOK {
		t.Fatalf("place = %d (%v)", status, res.Error)
	}
	var p struct {
		OrderID   string `json:"order_id"`
		PaymentID string `json:"payment_id"`
		Reference string `json:"reference"`
	}
	_ = json.Unmarshal(res.Data, &p)
	return jar, p.OrderID, p.PaymentID, p.Reference
}

func TestPortalMenuManagement(t *testing.T) {
	jar := loginAsStaff(t, "manager01@demo.biteorbit.local")
	status, res := env.Do(t, jar, "GET", "/api/v1/restaurant-portal/outlets", nil)
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	outletID := outlets[0]["id"].(string)

	// fetch menu
	status, res = env.Do(t, jar, "GET", "/api/v1/restaurant-portal/menu?outlet_id="+outletID, nil)
	if status != http.StatusOK {
		t.Fatalf("menu = %d", status)
	}
	var menu struct {
		Categories []struct {
			Items []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"items"`
		} `json:"categories"`
	}
	_ = json.Unmarshal(res.Data, &menu)
	if len(menu.Categories) == 0 || len(menu.Categories[0].Items) == 0 {
		t.Fatal("menu empty")
	}
	item := menu.Categories[0].Items[0]

	// toggle availability off
	status, res = env.Do(t, jar, "PATCH", "/api/v1/restaurant-portal/menu/items/"+item.ID, map[string]any{
		"is_available": false,
	})
	if status != http.StatusOK {
		t.Fatalf("toggle = %d (%v)", status, res.Error)
	}

	// customer menu must no longer offer it as available
	status, res = env.Do(t, env.NewJar(), "GET", "/api/v1/restaurants/"+outletID+"/menu", nil)
	var custMenu struct {
		Categories []struct {
			Items []struct {
				ID          string `json:"id"`
				IsAvailable bool   `json:"is_available"`
			} `json:"items"`
		} `json:"categories"`
	}
	_ = json.Unmarshal(res.Data, &custMenu)
	for _, c := range custMenu.Categories {
		for _, i := range c.Items {
			if i.ID == item.ID && i.IsAvailable {
				t.Fatal("customer menu still shows item available")
			}
		}
	}

	// price change is audited
	newPrice := int64(49900)
	status, _ = env.Do(t, jar, "PATCH", "/api/v1/restaurant-portal/menu/items/"+item.ID, map[string]any{
		"price_paise": newPrice,
	})
	if status != http.StatusOK {
		t.Fatalf("price update = %d", status)
	}
	var audits int
	_ = env.Pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM audit_logs WHERE target_type='menu_item' AND target_id=$1`, item.ID).Scan(&audits)
	if audits < 1 {
		t.Fatal("menu change not audited")
	}

	// invalid price rejected
	status, _ = env.Do(t, jar, "PATCH", "/api/v1/restaurant-portal/menu/items/"+item.ID, map[string]any{
		"price_paise": 5,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("bogus price accepted (%d)", status)
	}

	// restore availability
	_, _ = env.Do(t, jar, "PATCH", "/api/v1/restaurant-portal/menu/items/"+item.ID, map[string]any{
		"is_available": true,
	})
}

func TestPortalAnalytics(t *testing.T) {
	jar := loginAsStaff(t, "manager01@demo.biteorbit.local")
	status, res := env.Do(t, jar, "GET", "/api/v1/restaurant-portal/outlets", nil)
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	outletID := outlets[0]["id"].(string)

	status, res = env.Do(t, jar, "GET", "/api/v1/restaurant-portal/analytics?outlet_id="+outletID+"&days=30", nil)
	if status != http.StatusOK {
		t.Fatalf("analytics = %d (%v)", status, res.Error)
	}
	var a struct {
		Orders        int64  `json:"orders"`
		Delivered     int64  `json:"delivered"`
		GovPaise      int64  `json:"gov_paise"`
		NetSalesPaise int64  `json:"net_sales_paise"`
		AOVPaise      int64  `json:"aov_paise"`
		CancelRateBP  int64  `json:"cancel_rate_bp"`
		TopItems      []any  `json:"top_items"`
		DemoNote      string `json:"demo_note"`
	}
	_ = json.Unmarshal(res.Data, &a)
	if a.Orders == 0 {
		t.Fatal("seeded outlet should have orders in 30 days")
	}
	if a.Delivered > 0 && a.AOVPaise == 0 {
		t.Fatal("AOV missing with delivered orders")
	}
	if a.GovPaise == 0 {
		t.Fatal("GOV missing")
	}
	if a.DemoNote == "" {
		t.Fatal("analytics must carry the GOV definition note")
	}
}
