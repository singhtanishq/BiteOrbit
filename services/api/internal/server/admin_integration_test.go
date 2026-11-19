package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"
)

func loginAdmin(t *testing.T, email string) *cookiejar.Jar {
	t.Helper()
	jar := env.NewJar()
	status, res := env.Do(t, jar, "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Demo@1234",
	})
	if status != http.StatusOK {
		t.Fatalf("admin login = %d (%v)", status, res.Error)
	}
	return jar
}

func TestAdminOverview(t *testing.T) {
	jar := loginAdmin(t, "admin@demo.biteorbit.local")
	status, res := env.Do(t, jar, "GET", "/api/v1/admin/overview?days=30", nil)
	if status != http.StatusOK {
		t.Fatalf("overview = %d (%v)", status, res.Error)
	}
	var m struct {
		OrdersTotal    int64            `json:"orders_total"`
		OrdersByStatus map[string]int64 `json:"orders_by_status"`
		GovPaise       int64            `json:"gov_paise"`
		ActiveOutlets  int64            `json:"active_outlets"`
		DemoNote       string           `json:"demo_note"`
	}
	_ = json.Unmarshal(res.Data, &m)
	if m.OrdersTotal == 0 || m.GovPaise == 0 || m.ActiveOutlets == 0 {
		t.Fatalf("overview metrics empty: %+v", m)
	}
	if m.DemoNote == "" {
		t.Fatal("metrics must carry definition note")
	}

	// support agent can view but the RBAC guard blocks customer roles
	cj := env.NewJar()
	env.RegisterCustomerWithJar(t, cj, "admin-guard@example.test")
	status, _ = env.Do(t, cj, "GET", "/api/v1/admin/overview", nil)
	if status != http.StatusForbidden {
		t.Fatalf("customer on admin = %d, want 403", status)
	}
	// anonymous
	status, _ = env.Do(t, env.NewJar(), "GET", "/api/v1/admin/overview", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous on admin = %d, want 401", status)
	}
}

func TestAdminApplicationReview(t *testing.T) {
	adminJar := loginAdmin(t, "admin@demo.biteorbit.local")

	// create a pending organisation + application directly (demo data)
	var appID string
	err := env.Pool.QueryRow(t.Context(), `
		WITH org AS (
			INSERT INTO restaurant_organisations (legal_name, display_name, status)
			VALUES ('New Bites LLC (demo)', 'Chaat Chakra 2', 'submitted') RETURNING id
		), app AS (
			INSERT INTO restaurant_applications (organisation_id, status, applicant_note)
			SELECT id, 'submitted', 'Please review our demo outlet' FROM org RETURNING id
		)
		SELECT id FROM app`).Scan(&appID)
	if err != nil {
		t.Fatalf("seed application: %v", err)
	}

	// approve with note
	status, res := env.Do(t, adminJar, "POST", "/api/v1/admin/applications/"+appID+"/review",
		map[string]any{"decision": "approve", "note": "Documents verified in demo review"})
	if status != http.StatusOK {
		t.Fatalf("review = %d (%v)", status, res.Error)
	}
	var reviewed struct {
		Status    string `json:"status"`
		Simulated bool   `json:"simulated"`
	}
	_ = json.Unmarshal(res.Data, &reviewed)
	if reviewed.Status != "approved" || !reviewed.Simulated {
		t.Fatalf("review result: %+v", reviewed)
	}

	// org status follows
	var orgStatus string
	_ = env.Pool.QueryRow(t.Context(), `
		SELECT org.status FROM restaurant_organisations org
		JOIN restaurant_applications ra ON ra.organisation_id = org.id WHERE ra.id = $1`, appID).Scan(&orgStatus)
	if orgStatus != "approved" {
		t.Fatalf("org status = %s", orgStatus)
	}

	// audited
	var audits int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM audit_logs WHERE action='application.review' AND target_id=$1`, appID).Scan(&audits)
	if audits != 1 {
		t.Fatal("application review not audited")
	}

	// missing note rejected
	var appID2 string
	_ = env.Pool.QueryRow(t.Context(), `
		WITH org AS (
			INSERT INTO restaurant_organisations (legal_name, display_name, status)
			VALUES ('Reject Me LLC (demo)', 'Reject Me Grill', 'submitted') RETURNING id
		), app AS (
			INSERT INTO restaurant_applications (organisation_id, status) SELECT id, 'submitted' FROM org RETURNING id
		) SELECT id FROM app`).Scan(&appID2)
	status, _ = env.Do(t, adminJar, "POST", "/api/v1/admin/applications/"+appID2+"/review",
		map[string]any{"decision": "approve"})
	if status != http.StatusBadRequest {
		t.Fatalf("note-less review = %d, want 400", status)
	}
}

func TestAdminOrderCancelAudited(t *testing.T) {
	adminJar := loginAdmin(t, "admin@demo.biteorbit.local")
	jar, orderID, paymentID, _ := setupOnlineOrder(t, "admin-cancel@example.test", "mock_upi")
	_, _ = env.Do(t, jar, "POST", "/api/v1/payments/"+paymentID+"/simulate", map[string]any{"outcome": "success"})

	// admin cancels with mandatory reason
	status, res := env.Do(t, adminJar, "POST", "/api/v1/admin/orders/"+orderID+"/cancel",
		map[string]any{"reason": "Confirmed fraud pattern (demo escalation)"})
	if status != http.StatusOK {
		t.Fatalf("admin cancel = %d (%v)", status, res.Error)
	}

	// payment refunded + audit row
	var payStatus string
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&payStatus)
	if payStatus != "refunded" {
		t.Fatalf("payment after admin cancel = %s", payStatus)
	}
	var audits int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM audit_logs WHERE action='order.admin_cancel' AND target_id=$1`, orderID).Scan(&audits)
	if audits != 1 {
		t.Fatal("admin cancellation not audited")
	}

	// audit log endpoint shows it
	status, res = env.Do(t, adminJar, "GET", "/api/v1/admin/audit-logs?target_type=order&target_id="+orderID, nil)
	var logs []map[string]any
	_ = json.Unmarshal(res.Data, &logs)
	if status != http.StatusOK || len(logs) == 0 {
		t.Fatalf("audit endpoint = %d with %d rows", status, len(logs))
	}

	// support agent cannot cancel (permission refinement)
	agentJar := loginAdmin(t, "support@demo.biteorbit.local")
	jar2, orderID2, _, _ := setupOnlineOrder(t, "admin-cancel2@example.test", "mock_upi")
	_ = jar2
	status, _ = env.Do(t, agentJar, "POST", "/api/v1/admin/orders/"+orderID2+"/cancel",
		map[string]any{"reason": "agent attempt"})
	if status != http.StatusForbidden {
		t.Fatalf("agent cancel = %d, want 403", status)
	}
}

func TestAdminUserSuspension(t *testing.T) {
	adminJar := loginAdmin(t, "admin@demo.biteorbit.local")
	email := fmt.Sprintf("suspend-me-%d@example.test", time.Now().UnixNano())
	jar := env.NewJar()
	env.RegisterCustomerWithJar(t, jar, email)
	var userID string
	_ = env.Pool.QueryRow(t.Context(), `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)

	status, res := env.Do(t, adminJar, "POST", "/api/v1/admin/users/"+userID+"/suspend",
		map[string]any{"reason": "Abuse signals under review (demo)"})
	if status != http.StatusOK {
		t.Fatalf("suspend = %d (%v)", status, res.Error)
	}

	// sessions revoked — the user's jar no longer works
	status, _ = env.Do(t, jar, "GET", "/api/v1/auth/session", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("suspended user session still valid (%d)", status)
	}

	// login blocked
	status, _ = env.Do(t, env.NewJar(), "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Passw0rdLocal!",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("suspended user can log in (%d)", status)
	}

	// reactivation
	status, _ = env.Do(t, adminJar, "POST", "/api/v1/admin/users/"+userID+"/activate",
		map[string]any{"reason": "Appeal accepted (demo)"})
	if status != http.StatusOK {
		t.Fatalf("activate = %d", status)
	}
}

func TestAdminCouponManagement(t *testing.T) {
	adminJar := loginAdmin(t, "admin@demo.biteorbit.local")

	// create
	status, res := env.Do(t, adminJar, "POST", "/api/v1/admin/coupons", map[string]any{
		"code": "ADMIN10", "title": "Admin-created demo coupon",
		"discount_type": "percent", "percent_bp": 1000, "valid_days": 7,
		"usage_limit_per_user": 1, "min_order_paise": 29900,
	})
	if status != http.StatusCreated {
		t.Fatalf("create coupon = %d (%v)", status, res.Error)
	}
	var created struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	_ = json.Unmarshal(res.Data, &created)

	// usable by customers
	custJar, _, itemID := setupCheckout(t, "admin-coupon-user@example.test")
	_, _ = env.Do(t, custJar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 3,
	})
	status, res = env.Do(t, custJar, "POST", "/api/v1/checkout/coupon", map[string]any{"code": "ADMIN10"})
	var vc struct {
		Valid bool `json:"valid"`
	}
	_ = json.Unmarshal(res.Data, &vc)
	if !vc.Valid {
		t.Fatal("admin coupon not usable")
	}

	// deactivate with reason
	status, res = env.Do(t, adminJar, "PATCH", "/api/v1/admin/coupons/"+created.ID,
		map[string]any{"is_active": false, "reason": "Campaign ended early (demo)"})
	if status != http.StatusOK {
		t.Fatalf("deactivate = %d", status)
	}

	// audit row exists
	var audits int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM audit_logs WHERE action='coupon.updated' AND target_id=$1`, created.ID).Scan(&audits)
	if audits != 1 {
		t.Fatal("coupon change not audited")
	}
}

func TestAdminRiderManagement(t *testing.T) {
	adminJar := loginAdmin(t, "admin@demo.biteorbit.local")
	status, res := env.Do(t, adminJar, "GET", "/api/v1/admin/riders", nil)
	if status != http.StatusOK {
		t.Fatalf("riders = %d", status)
	}
	var riders []map[string]any
	_ = json.Unmarshal(res.Data, &riders)
	if len(riders) == 0 {
		t.Fatal("no riders")
	}
	riderID := riders[0]["id"].(string)

	// suspend + re-approve cycle
	status, _ = env.Do(t, adminJar, "POST", "/api/v1/admin/riders/"+riderID+"/suspend",
		map[string]any{"reason": "Demo suspension drill"})
	if status != http.StatusOK {
		t.Fatalf("suspend rider = %d", status)
	}
	var avail string
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT availability FROM delivery_partners WHERE id = $1`, riderID).Scan(&avail)
	if avail != "offline" {
		t.Fatalf("suspended rider still %s", avail)
	}
	status, _ = env.Do(t, adminJar, "POST", "/api/v1/admin/riders/"+riderID+"/approve",
		map[string]any{"reason": "Demo drill complete"})
	if status != http.StatusOK {
		t.Fatalf("re-approve = %d", status)
	}
}

func TestSupportTicketFlow(t *testing.T) {
	// customer creates an order-linked ticket
	email := "support-user@example.test"
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
		map[string]string{"Idempotency-Key": "support-" + email})
	var p struct {
		OrderID string `json:"order_id"`
	}
	_ = json.Unmarshal(res.Data, &p)

	status, res = env.Do(t, jar, "POST", "/api/v1/support/tickets", map[string]any{
		"category": "late_delivery", "subject": "Where is my order?",
		"body": "It has been longer than the estimate.", "order_id": p.OrderID,
	})
	if status != http.StatusCreated {
		t.Fatalf("create ticket = %d (%v)", status, res.Error)
	}
	var ticket struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(res.Data, &ticket)

	// customer cannot see the admin queue
	status, _ = env.Do(t, jar, "GET", "/api/v1/admin/support/tickets", nil)
	if status != http.StatusForbidden {
		t.Fatalf("customer on admin tickets = %d, want 403", status)
	}

	// agent replies publicly and internally
	adminJar := loginAdmin(t, "support@demo.biteorbit.local")
	status, _ = env.Do(t, adminJar, "POST", "/api/v1/admin/support/tickets/"+ticket.ID+"/reply",
		map[string]any{"body": "Internal: check rider allocation demo log.", "internal_note": true})
	if status != http.StatusCreated {
		t.Fatalf("internal reply = %d", status)
	}
	status, _ = env.Do(t, adminJar, "POST", "/api/v1/admin/support/tickets/"+ticket.ID+"/reply",
		map[string]any{"body": "We are sorry! The rider is en route (demo)."})
	if status != http.StatusCreated {
		t.Fatalf("public reply = %d", status)
	}

	// customer thread shows the public reply but NOT the internal note
	status, res = env.Do(t, jar, "GET", "/api/v1/support/tickets/"+ticket.ID, nil)
	var thread struct {
		Messages []struct {
			Body     string `json:"body"`
			Internal bool   `json:"is_internal_note"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(res.Data, &thread)
	if status != http.StatusOK || len(thread.Messages) != 2 {
		t.Fatalf("thread messages = %d (status %d)", len(thread.Messages), status)
	}
	for _, m := range thread.Messages {
		if m.Internal {
			t.Fatal("internal note leaked to customer")
		}
	}

	// agent resolves
	status, _ = env.Do(t, adminJar, "POST", "/api/v1/admin/support/tickets/"+ticket.ID+"/status",
		map[string]any{"status": "resolved"})
	if status != http.StatusOK {
		t.Fatalf("resolve = %d", status)
	}
}

func TestAdminFeatureFlags(t *testing.T) {
	adminJar := loginAdmin(t, "admin@demo.biteorbit.local")
	status, res := env.Do(t, adminJar, "GET", "/api/v1/admin/feature-flags", nil)
	var flags []struct {
		Key     string `json:"key"`
		Enabled bool   `json:"enabled"`
	}
	_ = json.Unmarshal(res.Data, &flags)
	if status != http.StatusOK || len(flags) == 0 {
		t.Fatalf("flags = %d, %d rows", status, len(flags))
	}

	// toggle scheduled_orders
	status, res = env.Do(t, adminJar, "PATCH", "/api/v1/admin/feature-flags/scheduled_orders",
		map[string]any{"enabled": true, "reason": "Demo flag drill"})
	if status != http.StatusOK {
		t.Fatalf("toggle = %d", status)
	}
	var updated struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.Unmarshal(res.Data, &updated)
	if !updated.Enabled {
		t.Fatal("flag not updated")
	}
	// restore
	_, _ = env.Do(t, adminJar, "PATCH", "/api/v1/admin/feature-flags/scheduled_orders",
		map[string]any{"enabled": false, "reason": "restore demo default"})
}
